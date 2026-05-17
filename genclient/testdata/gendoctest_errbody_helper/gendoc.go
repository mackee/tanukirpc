package gendoctest_errbody_helper

import (
	"errors"
	"net/http"

	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

type ErrorBody struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
	Code    string `json:"code,omitempty"`
}

func buildErrorBody(err error) ErrorBody {
	body := ErrorBody{Message: err.Error(), Status: http.StatusInternalServerError}
	var ews tanukirpc.ErrorWithStatus
	if errors.As(err, &ews) {
		body.Status = ews.Status()
	}
	var coded interface{ Code() string }
	if errors.As(err, &coded) {
		body.Code = coded.Code()
	}
	return body
}

// newRouter is a factory helper. WithErrorBody is registered inside it, so the
// gentypescript analyzer must recurse into the helper's return value to pick
// up the error type — the original implementation only inspected the direct
// NewRouter call.
func newRouter() *tanukirpc.Router[struct{}] {
	return tanukirpc.NewRouter(
		struct{}{},
		tanukirpc.WithErrorBody[struct{}](buildErrorBody),
	)
}

func testGendoc() {
	router := newRouter()
	type pingResponse struct {
		Message string `json:"message"`
	}
	router.Get("/ping", tanukirpc.NewHandler(
		func(ctx tanukirpc.Context[struct{}], _ struct{}) (*pingResponse, error) {
			return &pingResponse{Message: "pong"}, nil
		},
	))

	genclient.AnalyzeTarget(router)
}
