package oidc_test

import (
	"testing"

	"github.com/mackee/tanukirpc/auth/oidc"
)

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
