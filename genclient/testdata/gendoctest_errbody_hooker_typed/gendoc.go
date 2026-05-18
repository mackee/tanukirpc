package gendoctest_errbody_hooker_typed

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

// testGendoc registers a typed body via WithErrorHooker(NewErrorBodyHooker(...)).
// gentypescript must walk the hooker argument back to its ErrorHookerWithBody[E]
// implementation and emit the same ErrorResponse shape it would for a direct
// WithErrorBody call.
func testGendoc() {
	router := tanukirpc.NewRouter(
		struct{}{},
		tanukirpc.WithErrorHooker[struct{}](tanukirpc.NewErrorBodyHooker(buildErrorBody)),
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
