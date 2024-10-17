package tanukirpc_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/mackee/tanukirpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testCase struct {
	name    string
	router  http.Handler
	request *http.Request
	expect  func(*testing.T, *http.Response, error)
}

func (tc testCase) assert(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(tc.router)
	defer server.Close()

	su, err := url.Parse(server.URL)
	require.NoError(t, err)
	tc.request.URL.Scheme = su.Scheme
	tc.request.URL.Host = su.Host

	client := server.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Do(tc.request)
	tc.expect(t, resp, err)
}

func TestRouter(t *testing.T) {
	assert.True(t, true)

	store := map[string]string{}
	testCases := []testCase{
		{
			name:    "simple hello handler",
			router:  newHelloHandler(),
			request: newHelloHandlerRequest(t),
			expect:  helloHandlerExpect,
		},
		{
			name:    "static registry handler when exist",
			router:  newStaticRegistryHandler(),
			request: newStaticRegistryHandlerRequest(t, "42"),
			expect:  staticRegistryHandlerExpectWhenExist,
		},
		{
			name:    "static registry handler when not exist",
			router:  newStaticRegistryHandler(),
			request: newStaticRegistryHandlerRequest(t, "100"),
			expect:  staticRegistryHandlerExpectWhenNotExist,
		},
		{
			name:    "defer do handler",
			router:  newDeferDoHandler(store),
			request: newDeferDoHandlerRequest(t),
			expect:  deferDoHandlerExpect(store),
		},
		{
			name:    "raw body codec handler",
			router:  rawBodyCodecHandler(),
			request: rawBodyCodecHandlerRequest(t),
			expect:  rawBodyCodecHandlerExpect,
		},
		{
			name:    "raw body codec field reader handler",
			router:  rawBodyCodecFieldReaderHandler(),
			request: rawBodyCodecHandlerRequest(t),
			expect:  rawBodyCodecHandlerExpect,
		},
		{
			name:    "raw body codec reader handler",
			router:  rawBodyCodecReaderHandler(),
			request: rawBodyCodecHandlerRequest(t),
			expect:  rawBodyCodecHandlerExpect,
		},
		{
			name:    "raw body codec bytes handler",
			router:  rawBodyCodecBytesHandler(),
			request: rawBodyCodecHandlerRequest(t),
			expect:  rawBodyCodecHandlerExpect,
		},
		{
			name:    "error redirect handler",
			router:  errorRedirectHandler(),
			request: errorRedirectHandlerRequest(t),
			expect:  errorRedirectHandlerExpect,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.assert(t)
		})
	}
}

func newHelloHandler() http.Handler {
	type helloRequest struct {
		Name string `urlparam:"name"`
	}
	type helloResponse struct {
		Message string `json:"message"`
	}
	helloHandler := func(ctx tanukirpc.Context[struct{}], req helloRequest) (*helloResponse, error) {
		return &helloResponse{Message: "Hello, " + req.Name}, nil
	}
	router := tanukirpc.NewRouter(struct{}{})
	router.Get("/hello/{name}", tanukirpc.NewHandler(helloHandler))

	return router
}

func newHelloHandlerRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "/hello/world", nil)
	require.NoError(t, err)
	req.Header.Set("accept", "application/json")
	return req
}

func helloHandlerExpect(t *testing.T, resp *http.Response, err error) {
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	type helloResponse struct {
		Message string `json:"message"`
	}
	var body helloResponse
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "Hello, world", body.Message)
}

func newStaticRegistryHandler() http.Handler {
	db := map[int]string{42: "john"}
	type registry struct {
		db map[int]string
	}
	reg := &registry{db: db}
	type accountRequest struct {
		ID int `query:"id"`
	}
	type accountResponse struct {
		Name string `json:"name"`
	}
	accountHandler := func(ctx tanukirpc.Context[*registry], req accountRequest) (*accountResponse, error) {
		name, ok := ctx.Registry().db[req.ID]
		if !ok {
			return nil, tanukirpc.WrapErrorWithStatus(
				http.StatusNotFound,
				errors.New("account not found"),
			)
		}
		return &accountResponse{Name: name}, nil
	}
	router := tanukirpc.NewRouter(reg)
	router.Get("/account", tanukirpc.NewHandler(accountHandler))

	return router
}

func newStaticRegistryHandlerRequest(t *testing.T, id string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "/account?id="+id, nil)
	require.NoError(t, err)
	req.Header.Set("accept", "application/json")
	return req
}

func staticRegistryHandlerExpectWhenExist(t *testing.T, resp *http.Response, err error) {
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	type accountResponse struct {
		Name string `json:"name"`
	}
	var body accountResponse
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "john", body.Name)
}

func staticRegistryHandlerExpectWhenNotExist(t *testing.T, resp *http.Response, err error) {
	require.NoError(t, err)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	var em tanukirpc.ErrorMessage
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&em))
	assert.Equal(t, "account not found", em.Error.Message)
}

func newDeferDoHandler(store map[string]string) http.Handler {
	type deferDoResponse struct {
		Ok bool `json:"ok"`
	}
	deferDoHandler := func(ctx tanukirpc.Context[struct{}], req struct{}) (*deferDoResponse, error) {
		ctx.Defer(func() error {
			store["afterResponseDefer"] = store["beforeResponseDefer"] + "ok"
			return nil
		})
		ctx.Defer(func() error {
			store["beforeResponseDefer"] = "ok"
			return nil
		}, tanukirpc.DeferDoTimingBeforeResponse)
		return &deferDoResponse{Ok: true}, nil
	}
	router := tanukirpc.NewRouter(struct{}{})
	router.Use(middleware.Logger)
	router.Get("/defer", tanukirpc.NewHandler(deferDoHandler))

	return router
}

func newDeferDoHandlerRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "/defer", nil)
	require.NoError(t, err)
	req.Header.Set("accept", "application/json")
	return req
}

func deferDoHandlerExpect(store map[string]string) func(t *testing.T, resp *http.Response, err error) {
	return func(t *testing.T, resp *http.Response, err error) {
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		type deferDoResponse struct {
			Ok bool `json:"ok"`
		}
		var body deferDoResponse
		assert.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, true, body.Ok)

		assert.Equal(t, "ok", store["beforeResponseDefer"])
		assert.Equal(t, "okok", store["afterResponseDefer"])
	}
}

func rawBodyCodecHandler() http.Handler {
	type rawBodyRequest struct {
		Body []byte `rawbody:"true"`
	}
	h := func(ctx tanukirpc.Context[struct{}], req rawBodyRequest) ([]byte, error) {
		name := string(req.Body)
		return []byte("hello, " + name), nil
	}
	router := tanukirpc.NewRouter(struct{}{})
	router.Post("/raw", tanukirpc.NewHandler(h))

	return router
}

func rawBodyCodecHandlerRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "/raw", strings.NewReader("world"))
	require.NoError(t, err)
	return req
}

func rawBodyCodecHandlerExpect(t *testing.T, resp *http.Response, err error) {
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, "hello, world", string(body))
}

func rawBodyCodecFieldReaderHandler() http.Handler {
	type rawBodyRequest struct {
		Body io.ReadCloser `rawbody:"true"`
	}
	h := func(ctx tanukirpc.Context[struct{}], req rawBodyRequest) ([]byte, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		return append([]byte("hello, "), body...), nil
	}
	router := tanukirpc.NewRouter(struct{}{})
	router.Post("/raw", tanukirpc.NewHandler(h))

	return router
}

func rawBodyCodecReaderHandler() http.Handler {
	h := func(ctx tanukirpc.Context[struct{}], req io.ReadCloser) ([]byte, error) {
		body, err := io.ReadAll(req)
		if err != nil {
			return nil, err
		}
		return append([]byte("hello, "), body...), nil
	}
	router := tanukirpc.NewRouter(struct{}{})
	router.Post("/raw", tanukirpc.NewHandler(h))

	return router
}

func rawBodyCodecBytesHandler() http.Handler {
	h := func(ctx tanukirpc.Context[struct{}], req []byte) (io.Reader, error) {
		resp := append([]byte("hello, "), req...)
		return strings.NewReader(string(resp)), nil
	}
	router := tanukirpc.NewRouter(struct{}{})
	router.Post("/raw", tanukirpc.NewHandler(h))

	return router
}

func errorRedirectHandler() http.Handler {
	h := func(ctx tanukirpc.Context[struct{}], req struct{}) (struct{}, error) {
		return struct{}{}, tanukirpc.ErrorRedirectTo(http.StatusFound, "https://example.com")
	}
	router := tanukirpc.NewRouter(struct{}{})
	router.Get("/redirect", tanukirpc.NewHandler(h))

	return router
}

func errorRedirectHandlerRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "/redirect", nil)
	require.NoError(t, err)
	return req
}

func errorRedirectHandlerExpect(t *testing.T, resp *http.Response, err error) {
	require.NoError(t, err)

	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "https://example.com", resp.Header.Get("Location"))
}
