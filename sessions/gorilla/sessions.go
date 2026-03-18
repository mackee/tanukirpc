package gorilla

import (
	"errors"
	"fmt"
	"net/http"

	gorillasessions "github.com/gorilla/sessions"
	"github.com/mackee/tanukirpc/sessions"
)

type secureCookieDecodeError interface {
	IsDecode() bool
}

func wrapInvalidSessionError(err error) error {
	return fmt.Errorf("failed to get session: %w", sessions.NewInvalidSessionError(err))
}

func isSecureCookieDecodeError(err error) bool {
	var decodeErr secureCookieDecodeError
	return errors.As(err, &decodeErr) && decodeErr.IsDecode()
}

type gorillaStore struct {
	store       gorillasessions.Store
	sessionName string
}

type Option func(*gorillaStore)

func WithSessionName(name string) Option {
	return func(o *gorillaStore) {
		o.sessionName = name
	}
}

// NewStore returns a new Gorilla session store.
func NewStore(store gorillasessions.Store, opts ...Option) (sessions.Store, error) {
	gs := &gorillaStore{
		store:       store,
		sessionName: "session",
	}
	for _, opt := range opts {
		opt(gs)
	}

	return gs, nil
}

// GetAccessor returns an accessor.
func (s *gorillaStore) GetAccessor(req *http.Request) (sessions.Accessor, error) {
	session, err := s.store.Get(req, s.sessionName)
	if err != nil {
		if isSecureCookieDecodeError(err) {
			return nil, wrapInvalidSessionError(err)
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return &accessor{
		store:   s,
		session: session,
	}, nil
}

type accessor struct {
	store   *gorillaStore
	session *gorillasessions.Session
}

// Set sets a value to the session.
func (s *accessor) Set(key, value any) error {
	s.session.Values[key] = value
	return nil
}

// Get gets a value from the session.
func (s *accessor) Get(key string) (any, bool) {
	value, ok := s.session.Values[key]
	return value, ok
}

// Remove removes a value from the session.
func (s *accessor) Remove(key string) error {
	delete(s.session.Values, key)
	return nil
}

// Save saves the session.
func (s *accessor) Save(ctx sessions.ReqResp) error {
	if err := s.session.Save(ctx.Request(), ctx.Response()); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}
	return nil
}
