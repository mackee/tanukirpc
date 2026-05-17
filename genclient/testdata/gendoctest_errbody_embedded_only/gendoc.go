package gendoctest_errbody_embedded_only

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

type meta struct {
	Code string `json:"code"`
}

// ErrorBody only embeds meta. encoding/json flattens its contents at runtime,
// but gentypescript does not represent embedded fields, so the generated TS
// would collapse to an unusable shape.
// The embedded warning fires per field, and the embedded-only fatal fires
// at the type so users see both why each embedded field is dropped and
// why generation is refused.
type ErrorBody struct { // want `gentypescript: error body type .*\.ErrorBody contains only embedded fields`
	meta // want `gentypescript: error body has an embedded field "meta"`
}

func build(err error) ErrorBody {
	return ErrorBody{meta: meta{Code: "X"}}
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
