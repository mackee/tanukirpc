package oidc_test

import (
	"net/http"
	"testing"

	"github.com/mackee/tanukirpc/auth/oidc"
	"golang.org/x/oauth2"
)

func TestPKCE(t *testing.T) {
	idp := newFakeIdP(t)
	withoutPKCE := func(string) []oidc.HandlersOption[*flowRegistry] {
		return []oidc.HandlersOption[*flowRegistry]{oidc.WithoutPKCE[*flowRegistry]()}
	}

	t.Run("enabled by default", func(t *testing.T) {
		app := newApp(t, idp, nil)
		flow := startLogin(t, app, "", "")
		if got := flow.authorize.Query().Get("code_challenge_method"); got != "S256" {
			t.Fatalf("code_challenge_method = %q, want S256", got)
		}
		if resp := flow.callback(t, idp, app); resp.StatusCode != http.StatusFound {
			t.Fatalf("callback: status = %d, want %d", resp.StatusCode, http.StatusFound)
		}
	})

	t.Run("disabled by WithoutPKCE", func(t *testing.T) {
		app := newApp(t, idp, withoutPKCE)
		flow := startLogin(t, app, "", "")
		if flow.authorize.Query().Has("code_challenge") {
			t.Fatalf("code_challenge is sent: %s", flow.authorize)
		}
		if resp := flow.callback(t, idp, app); resp.StatusCode != http.StatusFound {
			t.Fatalf("callback: status = %d, want %d", resp.StatusCode, http.StatusFound)
		}
	})

	t.Run("wrong verifier is rejected by the provider", func(t *testing.T) {
		app := newApp(t, idp, nil)
		flow := startLogin(t, app, "", "")
		q := flow.authorize.Query()
		q.Set("code_challenge", oauth2.S256ChallengeFromVerifier(oauth2.GenerateVerifier()))
		flow.authorize.RawQuery = q.Encode()
		if resp := flow.callback(t, idp, app); resp.StatusCode == http.StatusFound {
			t.Fatalf("callback: status = %d, want an error", resp.StatusCode)
		}
	})

	t.Run("session started without PKCE is rejected", func(t *testing.T) {
		app := newAppWith(t, idp, withoutPKCE, nil)
		flow := startLogin(t, app, "", "")
		if resp := flow.callback(t, idp, app); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("callback: status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
	})

	t.Run("verifier cannot be reused", func(t *testing.T) {
		app := newApp(t, idp, nil)
		flow := startLogin(t, app, "", "")
		if resp := flow.callback(t, idp, app); resp.StatusCode != http.StatusFound {
			t.Fatalf("callback: status = %d, want %d", resp.StatusCode, http.StatusFound)
		}
		if resp := flow.callback(t, idp, app); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("second callback: status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
	})
}

func TestWithAuthCodeOptions(t *testing.T) {
	idp := newFakeIdP(t)
	app := newApp(t, idp, func(string) []oidc.HandlersOption[*flowRegistry] {
		return []oidc.HandlersOption[*flowRegistry]{
			oidc.WithAuthCodeOptions[*flowRegistry](
				oauth2.SetAuthURLParam("prompt", "select_account"),
				oauth2.SetAuthURLParam("hd", "example.com"),
				// Must not override the parameters set by Redirect.
				oauth2.SetAuthURLParam("nonce", "fixed"),
				oauth2.SetAuthURLParam("code_challenge", "fixed"),
			),
		}
	})
	flow := startLogin(t, app, "", "")
	q := flow.authorize.Query()
	for key, want := range map[string]string{"prompt": "select_account", "hd": "example.com"} {
		if got := q.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	for _, key := range []string{"nonce", "code_challenge"} {
		if q.Get(key) == "fixed" {
			t.Errorf("%s is overridden by WithAuthCodeOptions", key)
		}
	}
	if resp := flow.callback(t, idp, app); resp.StatusCode != http.StatusFound {
		t.Fatalf("callback: status = %d, want %d", resp.StatusCode, http.StatusFound)
	}
}
