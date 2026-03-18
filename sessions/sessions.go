package sessions

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrInvalidSession indicates that the session cookie exists but could not be decoded.
var ErrInvalidSession = errors.New("invalid session")

type invalidSessionError struct {
	err error
}

func (e *invalidSessionError) Error() string {
	return fmt.Sprintf("%s: %v", ErrInvalidSession, e.err)
}

func (e *invalidSessionError) Unwrap() []error {
	return []error{ErrInvalidSession, e.err}
}

// NewInvalidSessionError wraps an underlying session decode error.
func NewInvalidSessionError(err error) error {
	return &invalidSessionError{err: err}
}

// IsInvalidSessionError reports whether err represents an invalid session cookie.
func IsInvalidSessionError(err error) bool {
	return errors.Is(err, ErrInvalidSession)
}

// ReqResp is an interface for request and response. uses for SessionAccessor.
type ReqResp interface {
	Request() *http.Request
	Response() http.ResponseWriter
}

// Accessor is an interface for session access.
type Accessor interface {
	Set(key, value any) error
	Get(key string) (any, bool)
	Remove(key string) error
	Save(ctx ReqResp) error
}

type RegistryWithAccessor interface {
	Session() Accessor
}

type Store interface {
	GetAccessor(req *http.Request) (Accessor, error)
}
