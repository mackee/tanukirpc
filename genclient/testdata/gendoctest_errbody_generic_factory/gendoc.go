package gendoctest_errbody_generic_factory

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

// newRouter is a same-package *generic* factory. SSA wraps the returned router
// in an *ssa.ChangeType because of the type parameter, so the analyzer must
// unwrap value conversions before matching the NewRouter call.
func newRouter[Reg any](reg Reg) *tanukirpc.Router[Reg] {
	return tanukirpc.NewRouter(
		reg,
		tanukirpc.WithErrorBody[Reg](build),
	)
}

func testGendoc() {
	router := newRouter(struct{}{})
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
