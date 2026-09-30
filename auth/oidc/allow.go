package oidc

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/sessions"
)

// ErrNotAllowed is returned by an Allower to reject the authenticated user.
// Wrap it (e.g. fmt.Errorf("%w: ...", ErrNotAllowed)) to add detail for logging.
var ErrNotAllowed = errors.New("not allowed")

// Allower decides whether the user identified by the ID token is allowed to log in.
// Allow returns nil to allow, an error wrapping ErrNotAllowed to reject,
// and any other error to report an internal failure.
type Allower[Reg sessions.RegistryWithAccessor] interface {
	Allow(ctx tanukirpc.Context[Reg], idToken *oidc.IDToken) error
}

// AllowFunc is an adapter to use an ordinary function as an Allower.
type AllowFunc[Reg sessions.RegistryWithAccessor] func(ctx tanukirpc.Context[Reg], idToken *oidc.IDToken) error

// Allow calls f(ctx, idToken).
func (f AllowFunc[Reg]) Allow(ctx tanukirpc.Context[Reg], idToken *oidc.IDToken) error {
	return f(ctx, idToken)
}

// AllowAnyOf allows the user if any of fns allows the user.
// If none allows, it returns an internal error if any occurred, otherwise ErrNotAllowed.
// AllowAnyOf with no fns rejects everyone.
func AllowAnyOf[Reg sessions.RegistryWithAccessor](fns ...AllowFunc[Reg]) AllowFunc[Reg] {
	return func(ctx tanukirpc.Context[Reg], idToken *oidc.IDToken) error {
		var errs []error
		for _, fn := range fns {
			err := fn(ctx, idToken)
			if err == nil {
				return nil
			}
			errs = append(errs, err)
		}
		// Joining an internal error with ErrNotAllowed would make it look like a rejection.
		for _, err := range errs {
			if !errors.Is(err, ErrNotAllowed) {
				return err
			}
		}
		if len(errs) == 0 {
			return ErrNotAllowed
		}
		return errors.Join(errs...)
	}
}

// AllowAllOf allows the user only if all of fns allow the user.
// AllowAllOf with no fns allows everyone.
func AllowAllOf[Reg sessions.RegistryWithAccessor](fns ...AllowFunc[Reg]) AllowFunc[Reg] {
	return func(ctx tanukirpc.Context[Reg], idToken *oidc.IDToken) error {
		for _, fn := range fns {
			if err := fn(ctx, idToken); err != nil {
				return err
			}
		}
		return nil
	}
}

// AllowDomains allows users whose "hd" claim (Google Workspace hosted domain) is one of the domains.
// Note that it is not the domain of the email address; personal accounts such as gmail.com have no "hd" claim.
// AllowDomains with no domains rejects everyone.
func AllowDomains[Reg sessions.RegistryWithAccessor](domains ...string) AllowFunc[Reg] {
	return func(_ tanukirpc.Context[Reg], idToken *oidc.IDToken) error {
		var claims struct {
			Hd string `json:"hd"`
		}
		if err := idToken.Claims(&claims); err != nil {
			return fmt.Errorf("failed to parse claims: %w", err)
		}
		if !slices.Contains(domains, claims.Hd) {
			return fmt.Errorf("%w: domain %q", ErrNotAllowed, claims.Hd)
		}
		return nil
	}
}

// AllowEmails allows users whose verified "email" claim is one of the emails.
// It requires the "email" scope. Emails are compared case-insensitively,
// and for gmail.com, dots in the local part are ignored as Gmail does.
// AllowEmails with no emails rejects everyone.
func AllowEmails[Reg sessions.RegistryWithAccessor](emails ...string) AllowFunc[Reg] {
	allowed := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		allowed[normalizeEmail(email)] = struct{}{}
	}
	return func(_ tanukirpc.Context[Reg], idToken *oidc.IDToken) error {
		var claims struct {
			Email         string    `json:"email"`
			EmailVerified boolClaim `json:"email_verified"`
		}
		if err := idToken.Claims(&claims); err != nil {
			return fmt.Errorf("failed to parse claims: %w", err)
		}
		if !claims.EmailVerified {
			return fmt.Errorf("%w: email %q is not verified", ErrNotAllowed, claims.Email)
		}
		if _, ok := allowed[normalizeEmail(claims.Email)]; !ok {
			return fmt.Errorf("%w: email %q", ErrNotAllowed, claims.Email)
		}
		return nil
	}
}

func normalizeEmail(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	local, domain, ok := strings.Cut(email, "@")
	if !ok {
		return email
	}
	if domain == "gmail.com" {
		local = strings.ReplaceAll(local, ".", "")
	}
	return local + "@" + domain
}

// boolClaim accepts both a JSON boolean and a string "true"/"false",
// since some providers send email_verified as a string.
type boolClaim bool

func (b *boolClaim) UnmarshalJSON(data []byte) error {
	var v bool
	if err := json.Unmarshal(data, &v); err == nil {
		*b = boolClaim(v)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*b = boolClaim(s == "true")
	return nil
}
