// Package oidc provides handlers for OIDC authentication.
package oidc

import (
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/sessions"
	"golang.org/x/oauth2"
)

// Handlers is a set of handlers for OIDC authentication.
type Handlers[Reg sessions.RegistryWithAccessor] struct {
	defaultReferrer          string
	allowedDomains           []string
	oauth2Config             *oauth2.Config
	verifier                 *oidc.IDTokenVerifier
	referrerBaseURL          string
	successBehavior          func(tanukirpc.Context[Reg], *SuccessBehaviorInput) error
	unauthorizedBehavior     func(tanukirpc.Context[Reg]) error
	notAllowedDomainBehavior func(tanukirpc.Context[Reg]) error
}

// HandlersOption is an option for Handlers.
type HandlersOption[Reg sessions.RegistryWithAccessor] func(*Handlers[Reg])

// WithDefaultReferrer sets the default referrer.
func WithDefaultReferrer[Reg sessions.RegistryWithAccessor](referrer string) HandlersOption[Reg] {
	return func(a *Handlers[Reg]) {
		a.defaultReferrer = referrer
	}
}

// WithAllowedDomains sets the allowed domains.
func WithAllowedDomains[Reg sessions.RegistryWithAccessor](domains ...string) HandlersOption[Reg] {
	return func(a *Handlers[Reg]) {
		a.allowedDomains = domains
	}
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

// WithNotAllowedDomainBehavior sets the not allowed behavior.
func WithNotAllowedDomainBehavior[Reg sessions.RegistryWithAccessor](fn func(tanukirpc.Context[Reg]) error) HandlersOption[Reg] {
	return func(a *Handlers[Reg]) {
		a.notAllowedDomainBehavior = fn
	}
}

// NewHandlers creates a new Handlers.
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

func (a *Handlers[Reg]) referrer(req *http.Request) string {
	referrer := req.Referer()
	if referrer == "" && a.referrerBaseURL != "" {
		s := strings.TrimPrefix(referrer, a.referrerBaseURL)
		if !strings.HasPrefix("/", s) {
			s = "/" + s
		}
		referrer = s
	}
	return referrer
}

func (a *Handlers[Reg]) setReferrer(ctx tanukirpc.Context[Reg], name string) error {
	req := ctx.Request()
	if referrer := a.referrer(req); referrer != "" {
		if err := ctx.Registry().Session().Set(name, referrer); err != nil {
			return fmt.Errorf("failed to set referrer: %w", err)
		}
	}
	return nil
}

func (a *Handlers[Reg]) getReferrer(ctx tanukirpc.Context[Reg], name string) string {
	reg := ctx.Registry()
	referrer, ok := reg.Session().Get(name)
	if ok {
		reg.Session().Remove(name)
		return referrer.(string)
	}
	return a.defaultReferrer
}

// Redirect redirects to the OIDC provider.
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

	if err := a.setReferrer(ctx, "redirect_referrer"); err != nil {
		return struct{}{}, fmt.Errorf("failed to set referrer: %w", err)
	}

	if err := reg.Session().Save(ctx); err != nil {
		return struct{}{}, fmt.Errorf("failed to save session: %w", err)
	}

	return struct{}{}, tanukirpc.ErrorRedirectTo(http.StatusFound, a.oauth2Config.AuthCodeURL(state, oidc.Nonce(nonce)))
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

	token, err := a.oauth2Config.Exchange(ctx.Request().Context(), req.Code)
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
	type claims struct {
		Hd string `json:"hd"`
	}
	var idTokenClaims claims
	if err := idToken.Claims(&idTokenClaims); err != nil {
		return struct{}{}, fmt.Errorf("failed to parse claims: %w", err)
	}

	allowed := len(a.allowedDomains) == 0 || slices.Contains(a.allowedDomains, idTokenClaims.Hd)
	if !allowed {
		if a.notAllowedDomainBehavior != nil {
			return struct{}{}, a.notAllowedDomainBehavior(ctx)
		}
		return struct{}{}, tanukirpc.WrapErrorWithStatus(http.StatusForbidden, fmt.Errorf("domain not allowed"))
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
