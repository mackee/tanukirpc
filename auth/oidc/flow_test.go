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
// Like real providers, it verifies the PKCE code_verifier at the token endpoint
// if the authorize request carried a code_challenge.
type fakeIdP struct {
	*httptest.Server
	key           *rsa.PrivateKey
	nonce         string
	codeChallenge string
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
		if err := idp.verifyCodeVerifier(r.PostFormValue("code_verifier")); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":             "invalid_grant",
				"error_description": err.Error(),
			})
			return
		}
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

func (idp *fakeIdP) verifyCodeVerifier(verifier string) error {
	switch {
	case idp.codeChallenge == "" && verifier == "":
		return nil
	case idp.codeChallenge == "":
		return fmt.Errorf("code_verifier was sent without code_challenge")
	case verifier == "":
		return fmt.Errorf("code_verifier is required")
	}
	sum := sha256.Sum256([]byte(verifier))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != idp.codeChallenge {
		return fmt.Errorf("code_verifier does not match code_challenge")
	}
	return nil
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

type optionsFunc func(appURL string) []oidc.HandlersOption[*flowRegistry]

func newApp(t *testing.T, idp *fakeIdP, opts optionsFunc) *httptest.Server {
	t.Helper()
	return newAppWith(t, idp, opts, opts)
}

// newAppWith creates an app whose Redirect and Callback are served by Handlers with different options,
// which share the session store.
func newAppWith(t *testing.T, idp *fakeIdP, redirectOpts, callbackOpts optionsFunc) *httptest.Server {
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
	var rOpts, cOpts []oidc.HandlersOption[*flowRegistry]
	if redirectOpts != nil {
		rOpts = redirectOpts(appURL)
	}
	if callbackOpts != nil {
		cOpts = callbackOpts(appURL)
	}

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
	router.Get("/auth/redirect", tanukirpc.NewHandler(oidc.NewHandlers(oauth2Config, provider, rOpts...).Redirect))
	router.Get("/auth/callback", tanukirpc.NewHandler(oidc.NewHandlers(oauth2Config, provider, cOpts...).Callback))
	app.Config.Handler = router
	app.Start()
	t.Cleanup(app.Close)
	return app
}

type loginFlow struct {
	client    *http.Client
	authorize *url.URL
}

// startLogin requests Redirect and returns the authorize URL it redirects to.
func startLogin(t *testing.T, app *httptest.Server, referer, returnTo string) *loginFlow {
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
	return &loginFlow{client: client, authorize: authorize}
}

// callback lets the IdP accept the authorize request and requests Callback.
func (f *loginFlow) callback(t *testing.T, idp *fakeIdP, app *httptest.Server) *http.Response {
	t.Helper()
	q := f.authorize.Query()
	idp.nonce = q.Get("nonce")
	idp.codeChallenge = ""
	if q.Has("code_challenge") {
		if method := q.Get("code_challenge_method"); method != "S256" {
			t.Fatalf("code_challenge_method = %q, want S256", method)
		}
		idp.codeChallenge = q.Get("code_challenge")
	}

	resp, err := f.client.Get(fmt.Sprintf("%s/auth/callback?code=code&state=%s", app.URL, url.QueryEscape(q.Get("state"))))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

// login runs Redirect -> (IdP) -> Callback and returns the Location of the final redirect.
func login(t *testing.T, idp *fakeIdP, app *httptest.Server, referer, returnTo string) string {
	t.Helper()
	resp := startLogin(t, app, referer, returnTo).callback(t, idp, app)
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("callback: status = %d", resp.StatusCode)
	}
	return resp.Header.Get("Location")
}
