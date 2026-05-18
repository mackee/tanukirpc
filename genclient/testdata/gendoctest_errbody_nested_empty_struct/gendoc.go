package gendoctest_errbody_nested_empty_struct

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

// Marker is an empty struct used as a "tombstone" field on the error body.
// encoding/json renders it as `{}` on the wire. The generated TypeScript
// must render it as `{}` (not `undefined`) so the client type matches the
// runtime, and the field should be eligible as a `typeof === "object"`
// discriminator in `isErrorResponse`.
type Marker struct{}

type ErrorBody struct {
	Message  string `json:"message"`
	NotFound Marker `json:"not_found"`
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
