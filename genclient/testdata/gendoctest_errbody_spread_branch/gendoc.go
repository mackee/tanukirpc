package gendoctest_errbody_spread_branch

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

// buildOptions is a spread helper whose return paths disagree about whether
// the resulting router uses a custom error body. The analyzer must not simply
// concatenate elements across paths; runtime takes one branch, so the
// generated client may misrepresent the response. Warn at the spread site.
func buildOptions(typed bool) []tanukirpc.RouterOption[struct{}] {
	if typed {
		return []tanukirpc.RouterOption[struct{}]{
			tanukirpc.WithErrorBody[struct{}](build),
		}
	}
	return nil
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
