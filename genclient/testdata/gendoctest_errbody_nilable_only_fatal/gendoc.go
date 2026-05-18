package gendoctest_errbody_nilable_only_fatal

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

// ErrorBody only has required slice and map fields. encoding/json emits
// `null` for nil slices/maps when omitempty is absent, so neither field is
// a reliable discriminator — a marshaler that returns the zero value
// produces `{"errors": null, "details": null}` and the generated
// `isErrorResponse` would treat the response as a success. gentypescript
// rejects the body until the user adds a non-nilable required field.
type ErrorBody struct { // want `gentypescript: error body type .*\.ErrorBody has no required field that can discriminate it from a success response at runtime`
	Errors  []string          `json:"errors"`
	Details map[string]string `json:"details"`
}

func build(err error) ErrorBody {
	return ErrorBody{}
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
