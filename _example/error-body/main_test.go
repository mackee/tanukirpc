package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mackee/tanukirpc"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	reg := &Registry{users: map[string]*User{"1": {ID: "1", Name: "Alice"}}}
	router := tanukirpc.NewRouter(
		reg,
		tanukirpc.WithErrorBody[*Registry](buildErrorBody),
	)
	router.Get("/api/users/{id}", tanukirpc.NewHandler(getUserHandler))
	return httptest.NewServer(router)
}

func TestErrorBody_NotFound(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/users/999", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	// The wire shape is the marshaler's return value at the top level — no
	// {"error": ...} envelope.
	var body ErrorBody
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != "USER_NOT_FOUND" {
		t.Errorf("code: got %q, want %q", body.Code, "USER_NOT_FOUND")
	}
	if body.Status != http.StatusNotFound {
		t.Errorf("status field: got %d, want %d", body.Status, http.StatusNotFound)
	}
	if !strings.Contains(body.Message, "id=999") {
		t.Errorf("message: got %q, expected to contain id=999", body.Message)
	}
}

func TestErrorBody_Success(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/users/1", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body GetUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.User == nil || body.User.Name != "Alice" {
		t.Errorf("user: got %+v, want Alice", body.User)
	}
}
