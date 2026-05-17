package tanukirpc

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

type ErrorWithStatus interface {
	error
	Status() int
}

type errorWithStatus struct {
	status int
	err    error
}

func (e *errorWithStatus) Error() string {
	return e.err.Error()
}

func (e *errorWithStatus) Status() int {
	return e.status
}

func (e *errorWithStatus) Unwrap() error {
	return e.err
}

func WrapErrorWithStatus(status int, err error) error {
	return &errorWithStatus{status: status, err: err}
}

type ErrorWithRedirect interface {
	error
	Status() int
	Redirect() string
}

type errorWithRedirect struct {
	status   int
	redirect string
}

func (e *errorWithRedirect) Error() string {
	return fmt.Sprintf("redirect to %s", e.redirect)
}

func (e *errorWithRedirect) Status() int {
	return e.status
}

func (e *errorWithRedirect) Redirect() string {
	return e.redirect
}

func ErrorRedirectTo(status int, redirect string) error {
	return &errorWithRedirect{status: status, redirect: redirect}
}

type ErrorMessage struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Message string `json:"message"`
}

// ErrorBodyMarshaler builds the inner error body from a handler error.
// The returned value is encoded as the "error" field of the response body
// (i.e. the wire shape is {"error": <E>}). Used together with WithErrorBody.
type ErrorBodyMarshaler[E any] func(err error) E

// errorEnvelope is the on-the-wire shape of the error response body when a
// custom marshaler is registered via WithErrorBody. The default codec writes
// the marshaler's return value as the "error" field, preserving the same
// envelope structure as the default ErrorMessage.
type errorEnvelope struct {
	Error any `json:"error"`
}

type ErrorHooker interface {
	OnError(w http.ResponseWriter, req *http.Request, logger *slog.Logger, codec Codec, err error)
}

// DefaultErrorHooker returns the standard tanukirpc error hooker.
func DefaultErrorHooker() ErrorHooker {
	return &errorHooker{}
}

type errorHooker struct {
	marshalBody func(err error) any
}

func newErrorHookerWithMarshaler(m func(err error) any) ErrorHooker {
	return &errorHooker{marshalBody: m}
}

func (e *errorHooker) OnError(w http.ResponseWriter, req *http.Request, logger *slog.Logger, codec Codec, err error) {
	if ewr, ok := errors.AsType[ErrorWithRedirect](err); ok {
		http.Redirect(w, req, ewr.Redirect(), ewr.Status())
		return
	}
	if ews, ok := errors.AsType[ErrorWithStatus](err); ok {
		w.WriteHeader(ews.Status())
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		logger.ErrorContext(req.Context(), "ocurred internal server error", slog.Any("error", err))
	}
	var body any
	if e.marshalBody != nil {
		body = errorEnvelope{Error: e.marshalBody(err)}
	} else {
		body = ErrorMessage{Error: ErrorBody{Message: err.Error()}}
	}
	if err := codec.Encode(w, req, body); err != nil {
		logger.ErrorContext(req.Context(), "failed to encode error response", slog.Any("error", err))
	}
}
