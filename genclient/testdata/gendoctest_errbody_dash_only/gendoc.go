package gendoctest_errbody_dash_only

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

// Every field uses `json:"-"`. encoding/json emits {"error":{}} at runtime,
// which the generated TypeScript ErrorResponse cannot represent.
type ErrorBody struct { // want `gentypescript: error body type .*\.ErrorBody has no json-tagged fields`
	Secret string `json:"-"`
	Note   string `json:"-"`
}

func build(err error) ErrorBody {
	return ErrorBody{Secret: err.Error(), Note: "internal"}
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
