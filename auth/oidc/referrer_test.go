package oidc_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	gorillasessions "github.com/gorilla/sessions"
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/auth/oidc"
	tsessions "github.com/mackee/tanukirpc/sessions"
	"github.com/mackee/tanukirpc/sessions/gorilla"
	"golang.org/x/oauth2"
)

type flowRegistry struct {
	accessor tsessions.Accessor
}

func (r *flowRegistry) Session() tsessions.Accessor { return r.accessor }

// fakeIdP is a minimal OIDC provider that issues an ID token carrying the nonce of the last authorize request.
type fakeIdP struct {
	*httptest.Server
	key   *rsa.PrivateKey
	nonce string
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	idp := &fakeIdP{key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                idp.URL,
			"authorization_endpoint":                idp.URL + "/authorize",
			"token_endpoint":                        idp.URL + "/token",
			"jwks_uri":                              idp.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		enc := base64.RawURLEncoding.EncodeToString
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"kid": "k1",
				"n":   enc(key.N.Bytes()),
				"e":   enc(big.NewInt(int64(key.E)).Bytes()),
			}},
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     idp.signIDToken(t),
		})
	})
	idp.Server = httptest.NewServer(mux)
	t.Cleanup(idp.Close)
	return idp
}

func (idp *fakeIdP) signIDToken(t *testing.T) string {
	enc := base64.RawURLEncoding.EncodeToString
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1", "typ": "JWT"})
	payload, _ := json.Marshal(map[string]any{
		"iss":   idp.URL,
		"sub":   "subject",
		"aud":   "client",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
		"nonce": idp.nonce,
	})
	signingInput := enc(header) + "." + enc(payload)
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, idp.key, crypto.SHA256, sum[:])
	if err != nil {
		t.Error(err)
	}
	return signingInput + "." + enc(sig)
}

func newApp(t *testing.T, idp *fakeIdP, opts func(appURL string) []oidc.HandlersOption[*flowRegistry]) *httptest.Server {
	t.Helper()
	store, err := gorilla.NewStore(gorillasessions.NewCookieStore([]byte("0123456789abcdef0123456789abcdef")))
	if err != nil {
		t.Fatal(err)
	}
	provider, err := gooidc.NewProvider(t.Context(), idp.URL)
	if err != nil {
		t.Fatal(err)
	}

	app := httptest.NewUnstartedServer(nil)
	appURL := "http://" + app.Listener.Addr().String()
	oauth2Config := &oauth2.Config{
		ClientID:     "client",
		ClientSecret: "secret",
		RedirectURL:  appURL + "/auth/callback",
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{gooidc.ScopeOpenID},
	}
	h := oidc.NewHandlers(oauth2Config, provider, opts(appURL)...)

	router := tanukirpc.NewRouter(
		&flowRegistry{},
		tanukirpc.WithContextFactory(tanukirpc.NewContextHookFactory(func(w http.ResponseWriter, req *http.Request) (*flowRegistry, error) {
			accessor, err := store.GetAccessor(req)
			if err != nil {
				return nil, err
			}
			return &flowRegistry{accessor: accessor}, nil
		})),
	)
	router.Get("/auth/redirect", tanukirpc.NewHandler(h.Redirect))
	router.Get("/auth/callback", tanukirpc.NewHandler(h.Callback))
	app.Config.Handler = router
	app.Start()
	t.Cleanup(app.Close)
	return app
}

// login runs Redirect -> (IdP) -> Callback and returns the Location of the final redirect.
func login(t *testing.T, idp *fakeIdP, app *httptest.Server, referer, returnTo string) string {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	redirect := app.URL + "/auth/redirect"
	if returnTo != "" {
		redirect += "?return_to=" + url.QueryEscape(returnTo)
	}
	req, err := http.NewRequest(http.MethodGet, redirect, nil)
	if err != nil {
		t.Fatal(err)
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("redirect: status = %d", resp.StatusCode)
	}
	authorize, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	idp.nonce = authorize.Query().Get("nonce")

	callback := fmt.Sprintf("%s/auth/callback?code=code&state=%s", app.URL, url.QueryEscape(authorize.Query().Get("state")))
	resp, err = client.Get(callback)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("callback: status = %d", resp.StatusCode)
	}
	return resp.Header.Get("Location")
}

func TestLoginReturnsToReferrer(t *testing.T) {
	idp := newFakeIdP(t)

	tests := []struct {
		name        string
		withBaseURL bool
		referer     func(appURL string) string
		returnTo    string
		want        string
	}{
		{
			name:        "no referer falls back to default",
			withBaseURL: true,
			want:        "/",
		},
		{
			name:        "same-origin referer is normalized to path",
			withBaseURL: true,
			referer:     func(appURL string) string { return appURL + "/dashboard?x=1" },
			want:        "/dashboard?x=1",
		},
		{
			name:        "cross-origin referer is rejected",
			withBaseURL: true,
			referer:     func(string) string { return "https://evil.example/phish" },
			want:        "/",
		},
		{
			name:        "cross-origin referer is rejected without base URL",
			withBaseURL: false,
			referer:     func(string) string { return "https://evil.example/phish" },
			want:        "/",
		},
		{
			name:        "same-host referer is normalized to path without base URL",
			withBaseURL: false,
			referer:     func(appURL string) string { return appURL + "/dashboard" },
			want:        "/dashboard",
		},
		{
			name:        "same-origin referer with protocol-relative path is rejected",
			withBaseURL: true,
			referer:     func(appURL string) string { return appURL + "//evil.example/phish" },
			want:        "/",
		},
		{
			name:        "return_to wins over referer",
			withBaseURL: true,
			referer:     func(appURL string) string { return appURL + "/dashboard" },
			returnTo:    "/items/1?tab=a",
			want:        "/items/1?tab=a",
		},
		{
			name:        "absolute return_to is ignored",
			withBaseURL: true,
			referer:     func(appURL string) string { return appURL + "/dashboard" },
			returnTo:    "https://evil.example/phish",
			want:        "/dashboard",
		},
		{
			name:        "protocol-relative return_to is ignored",
			withBaseURL: true,
			returnTo:    "//evil.example/phish",
			want:        "/",
		},
		{
			name:        "backslash return_to is ignored",
			withBaseURL: true,
			returnTo:    "/\\evil.example/phish",
			want:        "/",
		},
		{
			name:        "return_to with control characters is ignored",
			withBaseURL: true,
			returnTo:    "/\t/evil.example/phish",
			want:        "/",
		},
		{
			name:        "relative return_to is ignored",
			withBaseURL: true,
			returnTo:    "evil.example/phish",
			want:        "/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newApp(t, idp, func(appURL string) []oidc.HandlersOption[*flowRegistry] {
				if !tt.withBaseURL {
					return nil
				}
				return []oidc.HandlersOption[*flowRegistry]{oidc.WithReferrerBaseURL[*flowRegistry](appURL)}
			})
			var referer string
			if tt.referer != nil {
				referer = tt.referer(app.URL)
			}
			if got := login(t, idp, app, referer, tt.returnTo); got != tt.want {
				t.Errorf("redirected to %q after login, want %q", got, tt.want)
			}
		})
	}
}
