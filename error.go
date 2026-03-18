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

type ErrorHooker interface {
	OnError(w http.ResponseWriter, req *http.Request, logger *slog.Logger, codec Codec, err error)
}

type errorHooker struct{}

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
	if err := codec.Encode(w, req, ErrorMessage{Error: ErrorBody{Message: err.Error()}}); err != nil {
		logger.ErrorContext(req.Context(), "failed to encode error response", slog.Any("error", err))
	}
}
