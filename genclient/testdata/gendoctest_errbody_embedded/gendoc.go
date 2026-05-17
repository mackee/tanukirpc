package gendoctest_errbody_embedded

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

type meta struct {
	Code string `json:"code"`
}

type secretMeta struct {
	Token string `json:"token"`
}

// ErrorBody embeds meta (warned) and secretMeta with json:"-" (silenced,
// because encoding/json omits it at runtime). encoding/json flattens
// untagged embedded struct fields at runtime, but gentypescript does not
// represent them in the generated TypeScript ErrorResponse.
type ErrorBody struct {
	meta             // want `gentypescript: error body has an embedded field "meta"`
	secretMeta `json:"-"`
	Message    string `json:"message"`
}

func build(err error) ErrorBody {
	return ErrorBody{
		meta:       meta{Code: "X"},
		secretMeta: secretMeta{Token: "internal"},
		Message:    err.Error(),
	}
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
