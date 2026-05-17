package gendoctest_errbody_spread

import (
	"errors"
	"net/http"

	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

type ErrorBody struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
}

func buildErrorBody(err error) ErrorBody {
	body := ErrorBody{Message: err.Error(), Status: http.StatusInternalServerError}
	var ews tanukirpc.ErrorWithStatus
	if errors.As(err, &ews) {
		body.Status = ews.Status()
	}
	return body
}

// buildOptions is the kind of factory a project might use to assemble shared
// router options. NewRouter is then called as `NewRouter(reg, buildOptions()...)`
// — in SSA the variadic argument is the call result rather than the synthetic
// varargs slice the compiler emits for inline option lists.
func buildOptions() []tanukirpc.RouterOption[struct{}] {
	return []tanukirpc.RouterOption[struct{}]{
		tanukirpc.WithErrorBody[struct{}](buildErrorBody),
	}
}

func testGendoc() {
	router := tanukirpc.NewRouter(struct{}{}, buildOptions()...)
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
