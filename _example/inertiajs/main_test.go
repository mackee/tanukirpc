package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mackee/tanukirpc/codec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHomePage(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Inertia", "true")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	page := decodePage(t, w)
	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, "Home", page.Component)
	assert.Equal(t, "/", page.URL)
	assert.Equal(t, "dev", page.Version)
	assert.Equal(t, "tanukirpc + Inertia.js", page.Props["projectName"])
	assert.EqualValues(t, 3, page.Props["taskCount"])
}

func TestTasksIndexPage(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	req.Header.Set("X-Inertia", "true")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	page := decodePage(t, w)
	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, "Tasks/Index", page.Component)
	tasks, ok := page.Props["tasks"].([]any)
	require.True(t, ok)
	require.Len(t, tasks, 3)
	first, ok := tasks[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "1", first["id"])
	assert.Equal(t, "Read the protocol", first["title"])
}

func TestTaskShowPage(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/tasks/1", nil)
	req.Header.Set("X-Inertia", "true")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	page := decodePage(t, w)
	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, "Tasks/Show", page.Component)
	task, ok := page.Props["task"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "1", task["id"])
}

func TestMissingTaskReturnsInertiaErrorPage(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/tasks/missing", nil)
	req.Header.Set("X-Inertia", "true")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	page := decodePage(t, w)
	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
	assert.Equal(t, "Error", page.Component)
	assert.EqualValues(t, http.StatusNotFound, page.Props["status"])
	assert.Contains(t, page.Props["message"], "task missing not found")
}

func TestCreateTaskRedirectsToTasks(t *testing.T) {
	reg := NewRegistry()
	router, err := NewRouter(reg)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{"title":"Write the example","notes":"Use the codec package."}`))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-Inertia", "true")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusSeeOther, w.Result().StatusCode)
	assert.Equal(t, "/tasks", w.Result().Header.Get("Location"))
	tasks := reg.listTasks()
	require.Len(t, tasks, 4)
	assert.Equal(t, "Write the example", tasks[3].Title)
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	router, err := NewRouter(NewRegistry())
	require.NoError(t, err)
	return router
}

func decodePage(t *testing.T, w *httptest.ResponseRecorder) codec.PageObject {
	t.Helper()
	assert.Equal(t, "application/json", w.Result().Header.Get("Content-Type"))
	assert.Equal(t, "true", w.Result().Header.Get("X-Inertia"))
	assert.Equal(t, "X-Inertia", w.Result().Header.Get("Vary"))
	var page codec.PageObject
	require.NoError(t, json.NewDecoder(w.Result().Body).Decode(&page))
	return page
}
