package gendoctest_errbody_cross_pkg

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
	"github.com/mackee/tanukirpc/testdata/gendoctest_errbody_cross_pkg/helper"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

func testGendoc() {
	router := helper.NewRouter() // want `gentypescript: could not statically determine`
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
