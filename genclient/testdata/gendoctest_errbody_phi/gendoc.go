package gendoctest_errbody_phi

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

type ErrorBody struct {
	Message string `json:"message"`
}

func build(err error) ErrorBody { return ErrorBody{Message: err.Error()} }

func testGendoc(typed bool) {
	type pingResponse struct {
		Message string `json:"message"`
	}
	// Both branches return a *tanukirpc.Router[struct{}], but lowered as
	// *ssa.Phi at the use site. The analyzer previously only matched
	// *ssa.Call, so it dropped both branches silently. With #1 fixed it must
	// warn at the AnalyzeTarget argument site.
	var router *tanukirpc.Router[struct{}]
	if typed {
		router = tanukirpc.NewRouter(struct{}{}, tanukirpc.WithErrorBody[struct{}](build))
	} else {
		router = tanukirpc.NewRouter(struct{}{})
	}
	router.Get("/ping", tanukirpc.NewHandler(
		func(ctx tanukirpc.Context[struct{}], _ struct{}) (*pingResponse, error) {
			return &pingResponse{Message: "pong"}, nil
		},
	))

	genclient.AnalyzeTarget(router) // want `gentypescript: could not statically determine`
}
