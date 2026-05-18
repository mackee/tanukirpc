package gendoctest_errbody_defined_pointer

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

// Detail is a regular struct.
type Detail struct {
	Code string `json:"code"`
}

// MaybeDetail is a defined pointer type. encoding/json emits `null` when the
// underlying pointer is nil. gentypescript must surface that nilability:
//   - the generated TS field should be optional (`extra?:` rather than
//     `extra:`)
//   - the field must NOT be used as an `isErrorResponse` discriminator, since
//     a `null` value on the wire would otherwise fail the presence check and
//     misclassify a valid error response as a success.
type MaybeDetail *Detail

type ErrorBody struct {
	Message string      `json:"message"`
	Extra   MaybeDetail `json:"extra"`
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
