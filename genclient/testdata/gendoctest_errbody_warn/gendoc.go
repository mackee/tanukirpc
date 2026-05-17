package gendoctest_errbody_warn

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

// globalOpt hides the RouterOption behind a package-level variable. The
// analyzer cannot follow the *ssa.UnOp(load global) value back to the
// originating WithErrorBody call, so it must surface a diagnostic rather than
// silently emit the default error shape and let runtime drift.
var globalOpt = tanukirpc.WithErrorBody[struct{}](build)

func testGendoc() {
	router := tanukirpc.NewRouter(struct{}{}, globalOpt) // want `gentypescript: could not statically determine`
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
