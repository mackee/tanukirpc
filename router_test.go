package tanukirpc_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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

	resp, err := server.Client().Do(tc.request)
	tc.expect(t, resp, err)
}

func TestRouter(t *testing.T) {
	assert.True(t, true)

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
		println("accountHandler: %v", req.ID)
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
	router.Use(middleware.Logger)
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
