package gendoctest_errbody_warn_spread

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

// globalOpts is a package-level slice. NewRouter(reg, globalOpts...) gives the
// analyzer a slice value it cannot statically expand (it's a *ssa.UnOp load of
// a global, not the synthetic varargs allocation). The analyzer must warn at
// the spread site instead of silently producing the default error shape.
var globalOpts = []tanukirpc.RouterOption[struct{}]{
	tanukirpc.WithErrorBody[struct{}](build),
}

func testGendoc() {
	router := tanukirpc.NewRouter(struct{}{}, globalOpts...) // want `gentypescript: could not statically determine`
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
