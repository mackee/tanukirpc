package gendoctest_errbody_with_chain

import (
	"net/http"

	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

type ErrorBody struct {
	Message string `json:"message"`
}

func build(err error) ErrorBody { return ErrorBody{Message: err.Error()} }

func noopMW(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
	})
}

// AnalyzeTarget receives a router cloned via .With(). The clone preserves
// the parent's errorHooker at runtime, so the analyzer must walk back to
// the receiver router to discover WithErrorBody, not just look at the With
// call result.
func testGendoc() {
	root := tanukirpc.NewRouter(
		struct{}{},
		tanukirpc.WithErrorBody[struct{}](build),
	)
	api := root.With(noopMW)
	type pingResponse struct {
		Message string `json:"message"`
	}
	api.Get("/ping", tanukirpc.NewHandler(
		func(ctx tanukirpc.Context[struct{}], _ struct{}) (*pingResponse, error) {
			return &pingResponse{Message: "pong"}, nil
		},
	))

	genclient.AnalyzeTarget(api)
}
