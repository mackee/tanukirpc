package tanukirpc

import (
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
	if ews, ok := err.(ErrorWithStatus); ok {
		w.WriteHeader(ews.Status())
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		logger.ErrorContext(req.Context(), "ocurred internal server error", slog.Any("error", err))
	}
	codec.Encode(w, req, ErrorMessage{Error: ErrorBody{Message: err.Error()}})
}
