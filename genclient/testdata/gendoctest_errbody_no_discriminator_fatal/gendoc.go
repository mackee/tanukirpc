package gendoctest_errbody_no_discriminator_fatal

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

// ErrorBody has only omitempty fields, so every field is rendered as
// optional on the TS side. With no required field left, the generated
// `isErrorResponse` predicate would collapse to `return true` and classify
// any non-null object — including successful responses — as an error.
// gentypescript rejects this until the user adds at least one required
// primitive (or non-pointer nested struct) field.
type ErrorBody struct { // want `gentypescript: error body type .*\.ErrorBody has no required field that can discriminate it from a success response at runtime`
	Message string `json:"message,omitempty"`
	Status  int    `json:"status,omitempty"`
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
