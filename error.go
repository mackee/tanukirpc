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

// ErrorBodyMarshaler builds the error response body from a handler error.
// The returned value is encoded as the entire response body, so the marshaler
// controls the top-level wire shape. Used together with WithErrorBody and
// NewErrorBodyHooker.
type ErrorBodyMarshaler[E any] func(err error) E

type ErrorHooker interface {
	OnError(w http.ResponseWriter, req *http.Request, logger *slog.Logger, codec Codec, err error)
}

// ErrorHookerWithBody is an ErrorHooker that declares the type of the error
// response body it emits. gentypescript reads E off the hooker passed to
// WithErrorHooker (or WithErrorBody) to emit the matching ErrorResponse
// TypeScript type and the isErrorResponse narrowing predicate.
//
// ErrorBodyType is a marker that returns the zero value of E at runtime; the
// generator only inspects its signature.
type ErrorHookerWithBody[E any] interface {
	ErrorHooker
	ErrorBodyType() E
}

// DefaultErrorHooker returns the standard tanukirpc error hooker.
func DefaultErrorHooker() ErrorHooker {
	return &errorHooker{}
}

// NewErrorBodyHooker returns an ErrorHookerWithBody[E] that encodes the
// marshaler's return value as the entire response body. WithErrorBody uses
// this internally; expose it directly when composing other hookers that need
// to delegate to the typed body shape (for example as the validation branch
// of inertiajs.NewErrorHooker).
func NewErrorBodyHooker[E any](marshaler ErrorBodyMarshaler[E]) ErrorHookerWithBody[E] {
	return &errorBodyHooker[E]{marshaler: marshaler}
}

type errorHooker struct{}

func (e *errorHooker) OnError(w http.ResponseWriter, req *http.Request, logger *slog.Logger, codec Codec, err error) {
	if !writeErrorStatus(w, req, logger, err) {
		return
	}
	if err := codec.Encode(w, req, ErrorMessage{Error: ErrorBody{Message: err.Error()}}); err != nil {
		logger.ErrorContext(req.Context(), "failed to encode error response", slog.Any("error", err))
	}
}

type errorBodyHooker[E any] struct {
	marshaler ErrorBodyMarshaler[E]
}

func (e *errorBodyHooker[E]) OnError(w http.ResponseWriter, req *http.Request, logger *slog.Logger, codec Codec, err error) {
	if !writeErrorStatus(w, req, logger, err) {
		return
	}
	if err := codec.Encode(w, req, e.marshaler(err)); err != nil {
		logger.ErrorContext(req.Context(), "failed to encode error response", slog.Any("error", err))
	}
}

func (e *errorBodyHooker[E]) ErrorBodyType() E {
	var zero E
	return zero
}

// writeErrorStatus handles the status-code and redirect side of error
// rendering shared between the default hooker and the typed body hooker.
// Returns false when the response has already been completed (redirect),
// signaling the caller to skip body encoding.
func writeErrorStatus(w http.ResponseWriter, req *http.Request, logger *slog.Logger, err error) bool {
	if ewr, ok := errors.AsType[ErrorWithRedirect](err); ok {
		http.Redirect(w, req, ewr.Redirect(), ewr.Status())
		return false
	}
	if ews, ok := errors.AsType[ErrorWithStatus](err); ok {
		w.WriteHeader(ews.Status())
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		logger.ErrorContext(req.Context(), "occurred internal server error", slog.Any("error", err))
	}
	return true
}
