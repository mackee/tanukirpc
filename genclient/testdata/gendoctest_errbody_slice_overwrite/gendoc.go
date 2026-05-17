package gendoctest_errbody_slice_overwrite

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

type ErrorBody struct {
	Message string `json:"message"`
}

func build(err error) ErrorBody { return ErrorBody{Message: err.Error()} }

// buildOptions builds a slice and conditionally overwrites the first slot.
// Runtime sees either WithErrorHooker or WithErrorBody depending on `typed`,
// but the analyzer used to flatten every Store into the same slot and report
// resolved=true, silently picking a synthetic last-wins value.
func buildOptions(typed bool) []tanukirpc.RouterOption[struct{}] {
	opts := []tanukirpc.RouterOption[struct{}]{
		tanukirpc.WithErrorHooker[struct{}](tanukirpc.DefaultErrorHooker()),
	}
	if typed {
		opts[0] = tanukirpc.WithErrorBody[struct{}](build)
	}
	return opts
}

func testGendoc() {
	router := tanukirpc.NewRouter(struct{}{}, buildOptions(true)...) // want `gentypescript: could not statically determine`
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
