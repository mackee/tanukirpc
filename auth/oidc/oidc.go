// Package oidc provides handlers for OIDC authentication.
package oidc

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/sessions"
	"golang.org/x/oauth2"
)

// Handlers is a set of handlers for OIDC authentication.
type Handlers[Reg sessions.RegistryWithAccessor] struct {
	defaultReferrer      string
	allowers             []Allower[Reg]
	oauth2Config         *oauth2.Config
	verifier             *oidc.IDTokenVerifier
	referrerBaseURL      string
	successBehavior      func(tanukirpc.Context[Reg], *SuccessBehaviorInput) error
	unauthorizedBehavior func(tanukirpc.Context[Reg]) error
	notAllowedBehavior   func(tanukirpc.Context[Reg]) error
	authCodeOptions      []oauth2.AuthCodeOption
	disablePKCE          bool
}

// HandlersOption is an option for Handlers.
type HandlersOption[Reg sessions.RegistryWithAccessor] func(*Handlers[Reg])

// WithDefaultReferrer sets the default referrer.
func WithDefaultReferrer[Reg sessions.RegistryWithAccessor](referrer string) HandlersOption[Reg] {
	return func(a *Handlers[Reg]) {
		a.defaultReferrer = referrer
	}
}

// WithAllowFunc adds a function that decides whether the authenticated user is allowed.
// The function returns nil to allow, an error wrapping ErrNotAllowed to reject,
// and any other error to report an internal failure.
// When specified multiple times (including WithAllowedDomains), all of them must allow the user.
// To allow the user when any of several conditions is met, use AllowAnyOf.
func WithAllowFunc[Reg sessions.RegistryWithAccessor](fn func(ctx tanukirpc.Context[Reg], idToken *oidc.IDToken) error) HandlersOption[Reg] {
	return func(a *Handlers[Reg]) {
		a.allowers = append(a.allowers, AllowFunc[Reg](fn))
	}
}

// WithAllowedDomains allows only users whose "hd" claim (Google Workspace hosted domain) is one of the domains.
// It is a shorthand of WithAllowFunc(AllowDomains(domains...)), except that it has no effect if no domains are given.
func WithAllowedDomains[Reg sessions.RegistryWithAccessor](domains ...string) HandlersOption[Reg] {
	if len(domains) == 0 {
		return func(*Handlers[Reg]) {}
	}
	return WithAllowFunc(AllowDomains[Reg](domains...))
}

// WithReferrerBaseURL sets the referrer base URL.
func WithReferrerBaseURL[Reg sessions.RegistryWithAccessor](url string) HandlersOption[Reg] {
	return func(a *Handlers[Reg]) {
		a.referrerBaseURL = url
	}
}

type SuccessBehaviorInput struct {
	RawIDToken string
	IDToken    *oidc.IDToken
}

// WithSuccessBehavior sets the success behavior.
func WithSuccessBehavior[Reg sessions.RegistryWithAccessor](fn func(tanukirpc.Context[Reg], *SuccessBehaviorInput) error) HandlersOption[Reg] {
	return func(a *Handlers[Reg]) {
		a.successBehavior = fn
	}
}

// WithUnauthorizedBehavior sets the unauthorized behavior.
func WithUnauthorizedBehavior[Reg sessions.RegistryWithAccessor](fn func(tanukirpc.Context[Reg]) error) HandlersOption[Reg] {
	return func(a *Handlers[Reg]) {
		a.unauthorizedBehavior = fn
	}
}

// WithUnauthorizedRedirect sets the unauthorized behavior to redirect to the specified URL.
func WithUnauthorizedRedirect[Reg sessions.RegistryWithAccessor](url string) HandlersOption[Reg] {
	return func(a *Handlers[Reg]) {
		a.unauthorizedBehavior = func(ctx tanukirpc.Context[Reg]) error {
			return tanukirpc.ErrorRedirectTo(http.StatusFound, url)
		}
	}
}

// WithNotAllowedBehavior sets the behavior when the user is rejected by WithAllowFunc or WithAllowedDomains.
func WithNotAllowedBehavior[Reg sessions.RegistryWithAccessor](fn func(tanukirpc.Context[Reg]) error) HandlersOption[Reg] {
	return func(a *Handlers[Reg]) {
		a.notAllowedBehavior = fn
	}
}

// WithNotAllowedDomainBehavior sets the not allowed behavior.
//
// Deprecated: Use WithNotAllowedBehavior instead.
func WithNotAllowedDomainBehavior[Reg sessions.RegistryWithAccessor](fn func(tanukirpc.Context[Reg]) error) HandlersOption[Reg] {
	return WithNotAllowedBehavior(fn)
}

// WithAuthCodeOptions adds options to the authorization request, such as
// oauth2.SetAuthURLParam("prompt", "select_account") or oauth2.SetAuthURLParam("hd", "example.com").
// Note that "hd" is only a hint to the provider; use WithAllowedDomains to enforce it.
// The options cannot override "state", "nonce" and the PKCE parameters set by Redirect.
func WithAuthCodeOptions[Reg sessions.RegistryWithAccessor](opts ...oauth2.AuthCodeOption) HandlersOption[Reg] {
	return func(a *Handlers[Reg]) {
		a.authCodeOptions = append(a.authCodeOptions, opts...)
	}
}

// WithoutPKCE disables PKCE, which is enabled by default.
// Use it only if the provider rejects PKCE, or if either Redirect or Callback is not used
// and the other one does not handle the code verifier.
func WithoutPKCE[Reg sessions.RegistryWithAccessor]() HandlersOption[Reg] {
	return func(a *Handlers[Reg]) {
		a.disablePKCE = true
	}
}

// NewHandlers creates a new Handlers.
// PKCE (S256) is enabled by default. The code verifier is stored in the session by Redirect
// and required by Callback, so both must be enabled or disabled together.
func NewHandlers[Reg sessions.RegistryWithAccessor](oauth2Config *oauth2.Config, provider *oidc.Provider, opts ...HandlersOption[Reg]) *Handlers[Reg] {
	verifier := provider.Verifier(&oidc.Config{ClientID: oauth2Config.ClientID})
	h := &Handlers[Reg]{
		defaultReferrer: "/",
		oauth2Config:    oauth2Config,
		verifier:        verifier,
	}

	for _, opt := range opts {
		opt(h)
	}

	return h
}

// referrer returns the path of the Referer header if it is same-origin, or "" otherwise.
// The origin is the one of WithReferrerBaseURL if specified, or the host of the request.
func (a *Handlers[Reg]) referrer(req *http.Request) string {
	ref, err := url.Parse(req.Referer())
	if err != nil || ref.Host == "" {
		return ""
	}
	if a.referrerBaseURL != "" {
		base, err := url.Parse(a.referrerBaseURL)
		if err != nil || !strings.EqualFold(ref.Scheme, base.Scheme) || !strings.EqualFold(ref.Host, base.Host) {
			return ""
		}
	} else if !strings.EqualFold(ref.Host, req.Host) {
		return ""
	}
	path := ref.RequestURI()
	if !isLocalPath(path) {
		return ""
	}
	return path
}

// returnTo returns the "return_to" query parameter if it is a local path, or "" otherwise.
func returnTo(req *http.Request) string {
	path := req.URL.Query().Get("return_to")
	if !isLocalPath(path) {
		return ""
	}
	return path
}

// isLocalPath reports whether path is an absolute path on the same origin.
// It rejects protocol-relative URLs such as "//evil.example" and "/\evil.example",
// and control characters that browsers strip before resolving the URL.
func isLocalPath(path string) bool {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return false
	}
	return !strings.ContainsFunc(path, func(r rune) bool {
		return r == '\\' || r < 0x20 || r == 0x7f
	})
}

func (a *Handlers[Reg]) setReferrer(ctx tanukirpc.Context[Reg], name string) error {
	req := ctx.Request()
	referrer := returnTo(req)
	if referrer == "" {
		referrer = a.referrer(req)
	}
	if referrer != "" {
		if err := ctx.Registry().Session().Set(name, referrer); err != nil {
			return fmt.Errorf("failed to set referrer: %w", err)
		}
	}
	return nil
}

func (a *Handlers[Reg]) getReferrer(ctx tanukirpc.Context[Reg], name string) string {
	reg := ctx.Registry()
	referrer, ok := reg.Session().Get(name)
	if !ok {
		return a.defaultReferrer
	}
	reg.Session().Remove(name)
	// Sessions saved by older versions may hold a cross-origin URL.
	if s, ok := referrer.(string); ok && isLocalPath(s) {
		return s
	}
	return a.defaultReferrer
}

// Redirect redirects to the OIDC provider.
// After a successful login, Callback redirects back to the "return_to" query parameter
// if it is a local path such as "/dashboard", or to the Referer header if it is same-origin,
// or to the default referrer.
func (a *Handlers[Reg]) Redirect(ctx tanukirpc.Context[Reg], _ struct{}) (_resp struct{}, err error) {
	reg := ctx.Registry()

	_state, err := uuid.NewRandom()
	if err != nil {
		return struct{}{}, fmt.Errorf("failed to generate state: %w", err)
	}
	state := _state.String()
	if err := reg.Session().Set("state", state); err != nil {
		return struct{}{}, fmt.Errorf("failed to set state: %w", err)
	}
	_nonce, err := uuid.NewRandom()
	if err != nil {
		return struct{}{}, fmt.Errorf("failed to generate nonce: %w", err)
	}
	nonce := _nonce.String()
	if err := reg.Session().Set("nonce", nonce); err != nil {
		return struct{}{}, fmt.Errorf("failed to set nonce: %w", err)
	}

	// The options set by Redirect come last so that authCodeOptions cannot override them.
	opts := append(append([]oauth2.AuthCodeOption{}, a.authCodeOptions...), oidc.Nonce(nonce))
	if !a.disablePKCE {
		verifier := oauth2.GenerateVerifier()
		if err := reg.Session().Set("pkce_verifier", verifier); err != nil {
			return struct{}{}, fmt.Errorf("failed to set pkce verifier: %w", err)
		}
		opts = append(opts, oauth2.S256ChallengeOption(verifier))
	}

	if err := a.setReferrer(ctx, "redirect_referrer"); err != nil {
		return struct{}{}, fmt.Errorf("failed to set referrer: %w", err)
	}

	if err := reg.Session().Save(ctx); err != nil {
		return struct{}{}, fmt.Errorf("failed to save session: %w", err)
	}

	return struct{}{}, tanukirpc.ErrorRedirectTo(http.StatusFound, a.oauth2Config.AuthCodeURL(state, opts...))
}

type AuthCallbackRequest struct {
	Code  string `query:"code"`
	State string `query:"state"`
}

// Callback handles the callback from the OIDC provider.
func (a *Handlers[Reg]) Callback(ctx tanukirpc.Context[Reg], req AuthCallbackRequest) (_resp struct{}, err error) {
	reg := ctx.Registry()
	state, ok := reg.Session().Get("state")
	if !ok {
		slog.WarnContext(ctx, "state not found")
		return struct{}{}, tanukirpc.WrapErrorWithStatus(http.StatusBadRequest, fmt.Errorf("request invalid"))
	}

	if req.State != state {
		slog.WarnContext(
			ctx,
			"state mismatch",
			slog.String("got", req.State),
			slog.Any("want", state),
		)
		return struct{}{}, tanukirpc.WrapErrorWithStatus(http.StatusBadRequest, fmt.Errorf("request invalid"))
	}

	var exchangeOpts []oauth2.AuthCodeOption
	if !a.disablePKCE {
		// Do not fall back to the exchange without PKCE; the session may have been
		// started before PKCE was enabled, which is rejected like a missing state.
		verifier, ok := reg.Session().Get("pkce_verifier")
		if !ok {
			slog.WarnContext(ctx, "pkce verifier not found")
			return struct{}{}, tanukirpc.WrapErrorWithStatus(http.StatusBadRequest, fmt.Errorf("request invalid"))
		}
		if err := reg.Session().Remove("pkce_verifier"); err != nil {
			return struct{}{}, fmt.Errorf("failed to remove pkce verifier: %w", err)
		}
		v, _ := verifier.(string)
		exchangeOpts = append(exchangeOpts, oauth2.VerifierOption(v))
	}

	token, err := a.oauth2Config.Exchange(ctx.Request().Context(), req.Code, exchangeOpts...)
	if err != nil {
		return struct{}{}, fmt.Errorf("failed to exchange code for token: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return struct{}{}, fmt.Errorf("no id_token in token response")
	}

	idToken, err := a.verifier.Verify(ctx.Request().Context(), rawIDToken)
	if err != nil {
		slog.WarnContext(ctx, "failed to verify id_token", slog.Any("error", err))
		return struct{}{}, tanukirpc.WrapErrorWithStatus(http.StatusBadRequest, fmt.Errorf("request invalid"))
	}
	nonce, ok := reg.Session().Get("nonce")
	if !ok {
		slog.WarnContext(ctx, "nonce not found")
		return struct{}{}, tanukirpc.WrapErrorWithStatus(http.StatusBadRequest, fmt.Errorf("request invalid"))
	}
	if idToken.Nonce != nonce {
		slog.WarnContext(
			ctx,
			"nonce mismatch",
			slog.String("got", idToken.Nonce),
			slog.Any("want", nonce),
		)
		return struct{}{}, tanukirpc.WrapErrorWithStatus(http.StatusBadRequest, fmt.Errorf("request invalid"))
	}
	if err := a.allow(ctx, idToken); err != nil {
		if !errors.Is(err, ErrNotAllowed) {
			return struct{}{}, fmt.Errorf("failed to check allowed: %w", err)
		}
		slog.WarnContext(ctx, "not allowed", slog.String("subject", idToken.Subject), slog.Any("error", err))
		if a.notAllowedBehavior != nil {
			return struct{}{}, a.notAllowedBehavior(ctx)
		}
		return struct{}{}, tanukirpc.WrapErrorWithStatus(http.StatusForbidden, ErrNotAllowed)
	}

	referrer := a.getReferrer(ctx, "redirect_referrer")

	if a.successBehavior != nil {
		input := &SuccessBehaviorInput{
			RawIDToken: rawIDToken,
			IDToken:    idToken,
		}
		if err := a.successBehavior(ctx, input); err != nil {
			return struct{}{}, fmt.Errorf("failed to run success behavior: %w", err)
		}
	} else {
		if err := reg.Session().Set("id_token", rawIDToken); err != nil {
			return struct{}{}, fmt.Errorf("failed to set id_token: %w", err)
		}
		if err := reg.Session().Save(ctx); err != nil {
			return struct{}{}, fmt.Errorf("failed to save session: %w", err)
		}
	}

	return struct{}{}, tanukirpc.ErrorRedirectTo(http.StatusFound, referrer)
}

func (a *Handlers[Reg]) allow(ctx tanukirpc.Context[Reg], idToken *oidc.IDToken) error {
	for _, allower := range a.allowers {
		if err := allower.Allow(ctx, idToken); err != nil {
			return err
		}
	}
	return nil
}

// Logout logs out the user.
func (a *Handlers[Reg]) Logout(ctx tanukirpc.Context[Reg], _ struct{}) (_resp struct{}, err error) {
	reg := ctx.Registry()
	if err := reg.Session().Remove("id_token"); err != nil {
		return struct{}{}, fmt.Errorf("failed to remove id_token: %w", err)
	}
	if err := reg.Session().Save(ctx); err != nil {
		return struct{}{}, fmt.Errorf("failed to save session: %w", err)
	}

	referrer := a.referrer(ctx.Request())
	if referrer == "" {
		referrer = a.defaultReferrer
	}

	return struct{}{}, tanukirpc.ErrorRedirectTo(http.StatusFound, referrer)
}

// Authorized checks if the user is authorized.
func (a *Handlers[Reg]) Authorized(ctx tanukirpc.Context[Reg]) error {
	reg := ctx.Registry()
	if _, ok := reg.Session().Get("id_token"); !ok {
		if a.unauthorizedBehavior != nil {
			return a.unauthorizedBehavior(ctx)
		}
		return tanukirpc.WrapErrorWithStatus(http.StatusUnauthorized, fmt.Errorf("unauthorized"))
	}

	return nil
}
