package gendoctest_errbody_ptr

import (
	"net/http"

	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

// ErrorBody as a pointer is registered with WithErrorBody. encoding/json can
// emit {"error": null} when build returns nil, but the generated TS treats
// `error` as a struct (not nullable). Warn so users prefer the value form.
type ErrorBody struct { // want `gentypescript: error body type \*.*\.ErrorBody is a pointer`
	Message string `json:"message"`
}

func build(err error) *ErrorBody {
	if err == nil {
		return nil
	}
	return &ErrorBody{Message: err.Error()}
}

func testGendoc() {
	router := tanukirpc.NewRouter(
		struct{}{},
		tanukirpc.WithErrorBody[struct{}](build),
	)
	type pingResponse struct {
		Message string `json:"message"`
	}
	router.Get("/ping", tanukirpc.NewHandler(
		func(ctx tanukirpc.Context[struct{}], _ struct{}) (*pingResponse, error) {
			return &pingResponse{Message: "pong"}, nil
		},
	))

	genclient.AnalyzeTarget(router)
	_ = http.StatusInternalServerError
}
