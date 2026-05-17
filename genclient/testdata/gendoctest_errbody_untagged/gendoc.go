package gendoctest_errbody_untagged

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

// ErrorBody has Code without a json tag. encoding/json would emit it as
// {"Code": "..."} alongside Message, but toFields skips untagged fields, so
// the generated TS silently drops Code. Warn so the user notices the gap.
type ErrorBody struct {
	Message string `json:"message"`
	Code    string // want `gentypescript: error body field "Code" has no json tag`
}

func build(err error) ErrorBody { return ErrorBody{Message: err.Error(), Code: "x"} }

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
}
