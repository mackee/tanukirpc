package codec

import (
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mackee/tanukirpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type inertiaTestProps struct {
	Title string   `json:"title"`
	Users []string `json:"users"`
}

func TestInertiajsEncodeJSONPage(t *testing.T) {
	codec := NewInertiajs(nil, WithAssetVersion("asset-v1"))
	req := httptest.NewRequest(http.MethodGet, "/users?page=2", nil)
	req.Header.Set("X-Inertia", "true")
	w := httptest.NewRecorder()

	err := codec.Encode(w, req, Render("Users/Index", inertiaTestProps{
		Title: "Users",
		Users: []string{"makoto"},
	}))
	require.NoError(t, err)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	assert.Equal(t, "true", resp.Header.Get("X-Inertia"))
	assert.Equal(t, "X-Inertia", resp.Header.Get("Vary"))

	var page PageObject
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&page))
	assert.Equal(t, "Users/Index", page.Component)
	assert.Equal(t, "/users?page=2", page.URL)
	assert.Equal(t, "asset-v1", page.Version)
	assert.Equal(t, "Users", page.Props["title"])
	assert.Equal(t, []any{"makoto"}, page.Props["users"])
	assert.Equal(t, map[string]any{}, page.Props["errors"])
}

func TestInertiajsEncodeJSONPageWithPageOptions(t *testing.T) {
	called := false
	codec := NewInertiajs(nil, WithAssetVersionFunc(func(req *http.Request) (any, error) {
		called = true
		return "asset-v1", nil
	}))
	req := httptest.NewRequest(http.MethodGet, "/reports", nil)
	req.Header.Set("X-Inertia", "true")
	w := httptest.NewRecorder()

	err := codec.Encode(w, req, Render(
		"Reports/Show",
		map[string]any{"title": "Report"},
		WithPageURL("/custom-report"),
		WithPageVersion("page-v1"),
		WithEncryptHistory(true),
		WithClearHistory(true),
		WithPreserveFragment(true),
	))
	require.NoError(t, err)

	var page PageObject
	require.NoError(t, json.NewDecoder(w.Result().Body).Decode(&page))
	assert.False(t, called)
	assert.Equal(t, "/custom-report", page.URL)
	assert.Equal(t, "page-v1", page.Version)
	assert.True(t, page.EncryptHistory)
	assert.True(t, page.ClearHistory)
	assert.True(t, page.PreserveFragment)
}

func TestInertiajsUsesAssetVersionFunc(t *testing.T) {
	codec := NewInertiajs(nil, WithAssetVersionFunc(func(req *http.Request) (any, error) {
		return req.Header.Get("X-Asset-Version"), nil
	}))
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.Header.Set("X-Inertia", "true")
	req.Header.Set("X-Asset-Version", "asset-from-request")
	w := httptest.NewRecorder()

	err := codec.Encode(w, req, Render("Dashboard", map[string]any{}))
	require.NoError(t, err)

	var page PageObject
	require.NoError(t, json.NewDecoder(w.Result().Body).Decode(&page))
	assert.Equal(t, "asset-from-request", page.Version)
}

func TestInertiajsEncodeHTMLPage(t *testing.T) {
	tmpl := template.Must(template.New("app").Parse(`<main>{{ .Page.Component }}</main><script type="application/json" data-page>{{ .PageJSON }}</script>`))
	codec := NewInertiajs(tmpl)
	req := httptest.NewRequest(http.MethodGet, "/danger", nil)
	w := httptest.NewRecorder()

	err := codec.Encode(w, req, Render("Danger/Show", struct {
		Title string `json:"title"`
	}{
		Title: "</script>",
	}))
	require.NoError(t, err)

	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
	assert.Equal(t, "X-Inertia", resp.Header.Get("Vary"))
	assert.Contains(t, string(body), "<main>Danger/Show</main>")
	assert.Contains(t, string(body), `\u003c/script\u003e`)
	assert.NotContains(t, string(body), `"</script>"`)
}

func TestInertiajsEncodeHTMLPageWithTemplateName(t *testing.T) {
	tmpl := template.Must(template.New("root").Parse(`{{ define "app" }}selected:{{ .Page.Component }}{{ end }}{{ define "other" }}wrong{{ end }}`))
	codec := NewInertiajs(tmpl, WithTemplateName("app"))
	req := httptest.NewRequest(http.MethodGet, "/named", nil)
	w := httptest.NewRecorder()

	err := codec.Encode(w, req, Render("Named/Template", map[string]any{}))
	require.NoError(t, err)

	body, err := io.ReadAll(w.Result().Body)
	require.NoError(t, err)
	assert.Equal(t, "selected:Named/Template", string(body))
}

func TestNormalizePropsPreservesErrors(t *testing.T) {
	props, err := normalizeProps(map[string]any{
		"errors": map[string]any{"name": "required"},
		"name":   "tanuki",
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]any{"name": "required"}, props["errors"])
	assert.Equal(t, "tanuki", props["name"])
}

func TestNormalizePropsRejectsNonObject(t *testing.T) {
	_, err := normalizeProps([]string{"not", "object"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JSON object")
}

func TestInertiajsIgnoresDecodeAndNonPageResponse(t *testing.T) {
	codec := NewInertiajs(nil)
	req := httptest.NewRequest(http.MethodGet, "/", strings.NewReader(""))

	assert.ErrorIs(t, codec.Decode(req, &struct{}{}), tanukirpc.ErrRequestNotSupportedAtThisCodec)
	assert.ErrorIs(t, codec.Encode(httptest.NewRecorder(), req, struct {
		Message string `json:"message"`
	}{Message: "ok"}), tanukirpc.ErrResponseNotSupportedAtThisCodec)
}

func TestInertiajsCodecListFallsThroughToDefaultCodecs(t *testing.T) {
	codecList := tanukirpc.CodecList{
		NewInertiajs(nil),
		tanukirpc.NewJSONCodec(),
	}

	t.Run("decode", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"tanuki"}`))
		req.Header.Set("Content-Type", "application/json")
		var got struct {
			Name string `json:"name"`
		}

		err := codecList.Decode(req, &got)
		require.NoError(t, err)
		assert.Equal(t, "tanuki", got.Name)
	})

	t.Run("encode", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()

		err := codecList.Encode(w, req, struct {
			Message string `json:"message"`
		}{Message: "ok"})
		require.NoError(t, err)

		var got struct {
			Message string `json:"message"`
		}
		require.NoError(t, json.NewDecoder(w.Result().Body).Decode(&got))
		assert.Equal(t, "ok", got.Message)
		assert.Equal(t, "application/json", w.Result().Header.Get("Content-Type"))
	})
}

func TestInertiaErrorHookerWritesInertiaErrorPage(t *testing.T) {
	codec := NewInertiajs(nil)
	hooker := NewInertiaErrorHooker(codec, func(req *http.Request, err error, status int) Page[map[string]any] {
		return Render("Errors/Show", map[string]any{
			"message": err.Error(),
			"status":  status,
		})
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.Header.Set("X-Inertia", "true")
	w := httptest.NewRecorder()

	hooker.OnError(w, req, logger, codec, tanukirpc.WrapErrorWithStatus(http.StatusNotFound, errors.New("missing")))

	resp := w.Result()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	assert.Equal(t, "true", resp.Header.Get("X-Inertia"))

	var page PageObject
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&page))
	assert.Equal(t, "Errors/Show", page.Component)
	assert.Equal(t, "missing", page.Props["message"])
	assert.EqualValues(t, http.StatusNotFound, page.Props["status"])
	assert.Equal(t, map[string]any{}, page.Props["errors"])
}

func TestInertiaErrorHookerWritesHTMLErrorPage(t *testing.T) {
	tmpl := template.Must(template.New("app").Parse(`<h1>{{ .Page.Component }}</h1><span>{{ index .Page.Props "status" }}</span>`))
	codec := NewInertiajs(tmpl)
	hooker := NewInertiaErrorHooker(codec, func(req *http.Request, err error, status int) Page[map[string]any] {
		return Render("Errors/HTML", map[string]any{
			"message": err.Error(),
			"status":  status,
		})
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	req := httptest.NewRequest(http.MethodGet, "/forbidden", nil)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()

	hooker.OnError(w, req, logger, codec, tanukirpc.WrapErrorWithStatus(http.StatusForbidden, errors.New("forbidden")))

	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
	assert.Equal(t, "X-Inertia", resp.Header.Get("Vary"))
	assert.Contains(t, string(body), "<h1>Errors/HTML</h1>")
	assert.Contains(t, string(body), "<span>403</span>")
}
