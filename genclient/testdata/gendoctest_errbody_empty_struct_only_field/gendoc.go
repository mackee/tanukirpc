package gendoctest_errbody_empty_struct_only_field

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

// Tombstone is an empty marker struct. Its presence on the wire (as `{}`)
// is what discriminates this error body — there are no other required
// fields. gentypescript must:
//   - Render the field as `tombstone: {};` (not `undefined`).
//   - Treat the field as a valid `typeof === "object"` discriminator so
//     the no-discriminator fatal does NOT fire.
type Tombstone struct{}

type ErrorBody struct {
	Code      string    `json:"code,omitempty"`
	Tombstone Tombstone `json:"tombstone"`
}

func build(err error) ErrorBody {
	_ = err
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
