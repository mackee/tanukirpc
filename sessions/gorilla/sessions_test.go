package gorilla

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	gorillasessions "github.com/gorilla/sessions"
	"github.com/mackee/tanukirpc/sessions"
	"github.com/stretchr/testify/require"
)

func TestGetAccessorMarksInvalidSession(t *testing.T) {
	t.Parallel()

	oldKey := []byte("0123456789abcdef0123456789abcdef")
	newKey := []byte("fedcba9876543210fedcba9876543210")

	issueStore := gorillasessions.NewCookieStore(oldKey)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	session, err := issueStore.Get(req, "session")
	require.NoError(t, err)
	session.Values["state"] = "value"
	require.NoError(t, session.Save(req, rec))

	resp := rec.Result()
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})
	cookies := resp.Cookies()
	require.Len(t, cookies, 1)

	store, err := NewStore(gorillasessions.NewCookieStore(newKey))
	require.NoError(t, err)

	invalidReq := httptest.NewRequest(http.MethodGet, "/", nil)
	invalidReq.AddCookie(cookies[0])

	a, err := store.GetAccessor(invalidReq)
	require.NotNil(t, a)
	require.Error(t, err)
	require.True(t, errors.Is(err, sessions.ErrInvalidSession))
	require.True(t, sessions.IsInvalidSessionError(err))

	acc, ok := a.(*accessor)
	require.True(t, ok)
	require.True(t, acc.session.IsNew)
}
