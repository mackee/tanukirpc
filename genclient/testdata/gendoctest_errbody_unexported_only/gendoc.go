package gendoctest_errbody_unexported_only

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

// Every field is unexported. encoding/json drops them at runtime even though
// they carry json tags, so the body actually serializes to {"error":{}}.
// The analyzer warns per field and refuses to generate.
type ErrorBody struct { // want `gentypescript: error body type .*\.ErrorBody has no json-tagged fields`
	message string `json:"message"` // want `gentypescript: error body field "message" is unexported but has a json tag`
	code    string `json:"code"`    // want `gentypescript: error body field "code" is unexported but has a json tag`
}

func build(err error) ErrorBody {
	return ErrorBody{message: err.Error(), code: "X"}
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
}
