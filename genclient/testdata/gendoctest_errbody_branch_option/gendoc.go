package gendoctest_errbody_branch_option

import (
	"net/http"

	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

type ErrorBody struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
}

func build(err error) ErrorBody {
	return ErrorBody{Message: err.Error(), Status: http.StatusInternalServerError}
}

// appOpt has two return paths: one installs the custom error body, the other
// fully replaces rendering with a custom hooker. The TS client cannot reflect
// both shapes; the analyzer keeps the optimistic ErrorResponse (first-wins)
// but must warn at the option site so users notice the mismatch.
func appOpt(typed bool) tanukirpc.RouterOption[struct{}] {
	if typed {
		return tanukirpc.WithErrorBody[struct{}](build)
	}
	return tanukirpc.WithErrorHooker[struct{}](tanukirpc.DefaultErrorHooker())
}

func testGendoc() {
	router := tanukirpc.NewRouter(
		struct{}{},
		appOpt(true), // want `gentypescript: could not statically determine`
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
