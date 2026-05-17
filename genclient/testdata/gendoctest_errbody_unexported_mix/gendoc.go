package gendoctest_errbody_unexported_mix

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

// Mixed body: Message is exported and renderable, secret carries a json
// tag but is unexported so encoding/json drops it. The analyzer warns on
// secret and the generated TypeScript omits it.
type ErrorBody struct {
	Message string `json:"message"`
	secret  string `json:"secret"` // want `gentypescript: error body field "secret" is unexported but has a json tag`
}

func build(err error) ErrorBody {
	return ErrorBody{Message: err.Error(), secret: "internal"}
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
