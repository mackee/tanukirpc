package gendoctest_errbody_wrapper

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

// appErrorBody is a project-level helper that wraps WithErrorBody. Runtime
// installs the marshaler exactly as if WithErrorBody had been called directly,
// so the analyzer must recurse into the helper's return values to discover the
// underlying WithErrorBody invocation.
func appErrorBody[Reg any]() tanukirpc.RouterOption[Reg] {
	return tanukirpc.WithErrorBody[Reg](buildErrorBody)
}

func testGendoc() {
	router := tanukirpc.NewRouter(
		struct{}{},
		appErrorBody[struct{}](),
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
