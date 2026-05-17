package gendoctest_errbody_multi

import (
	"net/http"

	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

type ErrorBody struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
}

func build(err error) ErrorBody {
	return ErrorBody{Message: err.Error(), Status: http.StatusInternalServerError}
}

// Two AnalyzeTarget calls cannot coexist: the generator emits a single client
// with a single ErrorResponse, so the analyzer rejects this configuration
// rather than silently picking one router's error body and applying it to the
// other's routes.
func testGendoc() {
	type pingResponse struct {
		Message string `json:"message"`
	}

	routerA := tanukirpc.NewRouter(struct{}{})
	routerA.Get("/a/ping", tanukirpc.NewHandler(
		func(ctx tanukirpc.Context[struct{}], _ struct{}) (*pingResponse, error) {
			return &pingResponse{Message: "pong"}, nil
		},
	))

	routerB := tanukirpc.NewRouter(
		struct{}{},
		tanukirpc.WithErrorBody[struct{}](build),
	)
	routerB.Get("/b/ping", tanukirpc.NewHandler(
		func(ctx tanukirpc.Context[struct{}], _ struct{}) (*pingResponse, error) {
			return &pingResponse{Message: "pong"}, nil
		},
	))

	genclient.AnalyzeTarget(routerA) // want `gentypescript: AnalyzeTarget must be called at most once per package`
	genclient.AnalyzeTarget(routerB) // want `gentypescript: AnalyzeTarget must be called at most once per package`
}
