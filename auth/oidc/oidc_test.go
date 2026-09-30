package oidc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/sessions"
)

type testRegistry struct{}

func (testRegistry) Session() sessions.Accessor { return nil }

const testIssuer = "https://issuer.example.com"

func newTestIDToken(t *testing.T, claims map[string]any) *oidc.IDToken {
	t.Helper()
	claims["iss"] = testIssuer
	claims["sub"] = "subject"
	claims["aud"] = "client"
	claims["exp"] = time.Now().Add(time.Hour).Unix()
	header, err := json.Marshal(map[string]string{"alg": "RS256"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	raw := enc(header) + "." + enc(payload) + "." + enc([]byte("signature"))

	verifier := oidc.NewVerifier(testIssuer, &oidc.StaticKeySet{}, &oidc.Config{
		ClientID:                   "client",
		InsecureSkipSignatureCheck: true,
	})
	idToken, err := verifier.Verify(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	return idToken
}

func TestHandlersAllow(t *testing.T) {
	type Reg = testRegistry
	domainOrEmail := WithAllowFunc(AllowAnyOf(
		AllowDomains[Reg]("example.com"),
		AllowEmails[Reg]("Alice@example.org", "first.last@gmail.com"),
	))
	errInternal := errors.New("internal")
	internal := func(tanukirpc.Context[Reg], *oidc.IDToken) error { return errInternal }
	deny := func(tanukirpc.Context[Reg], *oidc.IDToken) error { return fmt.Errorf("%w: deny", ErrNotAllowed) }
	allow := func(tanukirpc.Context[Reg], *oidc.IDToken) error { return nil }

	tests := []struct {
		name    string
		opts    []HandlersOption[Reg]
		claims  map[string]any
		wantErr error
	}{
		{
			name:   "no option allows everyone",
			claims: map[string]any{"hd": "other.com"},
		},
		{
			name:   "empty domains allows everyone",
			opts:   []HandlersOption[Reg]{WithAllowedDomains[Reg]()},
			claims: map[string]any{"hd": "other.com"},
		},
		{
			name:   "allowed domain",
			opts:   []HandlersOption[Reg]{WithAllowedDomains[Reg]("example.com", "example.net")},
			claims: map[string]any{"hd": "example.net"},
		},
		{
			name:    "not allowed domain",
			opts:    []HandlersOption[Reg]{WithAllowedDomains[Reg]("example.com")},
			claims:  map[string]any{"hd": "other.com"},
			wantErr: ErrNotAllowed,
		},
		{
			name:    "missing hd",
			opts:    []HandlersOption[Reg]{WithAllowedDomains[Reg]("example.com")},
			claims:  map[string]any{},
			wantErr: ErrNotAllowed,
		},
		{
			name:   "or: domain matches",
			opts:   []HandlersOption[Reg]{domainOrEmail},
			claims: map[string]any{"hd": "example.com", "email": "bob@example.com"},
		},
		{
			name:   "or: email matches",
			opts:   []HandlersOption[Reg]{domainOrEmail},
			claims: map[string]any{"email": "alice@example.org", "email_verified": true},
		},
		{
			name:    "or: unverified email",
			opts:    []HandlersOption[Reg]{domainOrEmail},
			claims:  map[string]any{"email": "alice@example.org", "email_verified": false},
			wantErr: ErrNotAllowed,
		},
		{
			name:    "or: neither matches",
			opts:    []HandlersOption[Reg]{domainOrEmail},
			claims:  map[string]any{"hd": "other.com", "email": "bob@other.com", "email_verified": true},
			wantErr: ErrNotAllowed,
		},
		{
			name:   "email: case-insensitive",
			opts:   []HandlersOption[Reg]{domainOrEmail},
			claims: map[string]any{"email": "ALICE@Example.org", "email_verified": true},
		},
		{
			name:   "email: gmail ignores dots",
			opts:   []HandlersOption[Reg]{domainOrEmail},
			claims: map[string]any{"email": "firstlast@gmail.com", "email_verified": true},
		},
		{
			name:    "email: dots are significant outside gmail",
			opts:    []HandlersOption[Reg]{WithAllowFunc(AllowEmails[Reg]("a.b@example.org"))},
			claims:  map[string]any{"email": "ab@example.org", "email_verified": true},
			wantErr: ErrNotAllowed,
		},
		{
			name:   "email: email_verified as string",
			opts:   []HandlersOption[Reg]{domainOrEmail},
			claims: map[string]any{"email": "alice@example.org", "email_verified": "true"},
		},
		{
			name:    "email: missing email",
			opts:    []HandlersOption[Reg]{domainOrEmail},
			claims:  map[string]any{"email_verified": true},
			wantErr: ErrNotAllowed,
		},
		{
			name:    "any of: empty rejects",
			opts:    []HandlersOption[Reg]{WithAllowFunc(AllowAnyOf[Reg]())},
			claims:  map[string]any{},
			wantErr: ErrNotAllowed,
		},
		{
			name:    "any of: internal error wins over rejection",
			opts:    []HandlersOption[Reg]{WithAllowFunc(AllowAnyOf(deny, internal))},
			claims:  map[string]any{},
			wantErr: errInternal,
		},
		{
			name:   "any of: allow wins over internal error",
			opts:   []HandlersOption[Reg]{WithAllowFunc(AllowAnyOf(internal, allow))},
			claims: map[string]any{},
		},
		{
			name:   "all of: empty allows",
			opts:   []HandlersOption[Reg]{WithAllowFunc(AllowAllOf[Reg]())},
			claims: map[string]any{},
		},
		{
			name:    "all of: one rejects",
			opts:    []HandlersOption[Reg]{WithAllowFunc(AllowAllOf(allow, deny))},
			claims:  map[string]any{},
			wantErr: ErrNotAllowed,
		},
		{
			name: "and: multiple options must all allow",
			opts: []HandlersOption[Reg]{
				WithAllowedDomains[Reg]("example.com"),
				WithAllowFunc(deny),
			},
			claims:  map[string]any{"hd": "example.com"},
			wantErr: ErrNotAllowed,
		},
		{
			name:    "internal error is propagated",
			opts:    []HandlersOption[Reg]{WithAllowFunc(internal)},
			claims:  map[string]any{},
			wantErr: errInternal,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handlers[Reg]{}
			for _, opt := range tt.opts {
				opt(h)
			}
			err := h.allow(nil, newTestIDToken(t, tt.claims))
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == errInternal && errors.Is(err, ErrNotAllowed) {
				t.Fatalf("internal error must not be ErrNotAllowed: %v", err)
			}
		})
	}
}
