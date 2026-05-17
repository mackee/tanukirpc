package gendoctest_errbody_struct_empty

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

// ErrorBody is the zero-field struct. encoding/json emits {"error":{}} at
// runtime, which the generated TypeScript ErrorResponse cannot represent.
type ErrorBody struct { // want `gentypescript: error body type .*\.ErrorBody has no json-tagged fields`
}

func build(err error) ErrorBody { return ErrorBody{} }

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
