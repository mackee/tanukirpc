package tanukirpc

import "net/http"

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
	Error string `json:"error"`
}

type ErrorHooker interface {
	OnError(w http.ResponseWriter, req *http.Request, codec Codec, err error)
}

type errorHooker struct{}

func (e *errorHooker) OnError(w http.ResponseWriter, req *http.Request, codec Codec, err error) {
	if ews, ok := err.(ErrorWithStatus); ok {
		w.WriteHeader(ews.Status())
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
	codec.Encode(w, req, ErrorMessage{Error: err.Error()})
}
