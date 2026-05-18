package gendoctest_errbody_any_fatal

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

// ErrorBody has an `any` field. encoding/json emits whatever concrete value
// sits in the interface at runtime, so the wire shape is undefined to a
// static analyzer. gentypescript rejects the body until the user declares
// the wire shape via a tstype tag.
type ErrorBody struct {
	Message string `json:"message"`
	Extra   any    `json:"extra"` // want `gentypescript: error body field has interface type any`
}

func build(err error) ErrorBody {
	return ErrorBody{Message: err.Error()}
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
