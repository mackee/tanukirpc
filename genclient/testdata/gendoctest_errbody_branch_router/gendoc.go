package gendoctest_errbody_branch_router

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

// newRouter has two return paths; only one installs the custom error body.
// At runtime which branch fires depends on `typed`, but the generated TS
// can only commit to one shape. The analyzer keeps the optimistic ErrorResponse
// (first-wins) but must warn so users notice the mismatch.
func newRouter(typed bool) *tanukirpc.Router[struct{}] {
	if typed {
		return tanukirpc.NewRouter(
			struct{}{},
			tanukirpc.WithErrorBody[struct{}](build),
		)
	}
	return tanukirpc.NewRouter(struct{}{})
}

func testGendoc() {
	router := newRouter(true) // want `gentypescript: could not statically determine`
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
