package gendoctest_errbody_override

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

// testGendoc registers WithErrorBody first and then WithErrorHooker. Per
// Router.apply's option order, WithErrorHooker wins (it overwrites the same
// errorHooker slot), so the runtime no longer emits {"error": <ErrorBody>}.
// The analyzer must mirror this: the generated client.ts should fall back to
// the default `{ error: { message: string } }` shape rather than emitting
// ErrorResponse for a body the runtime will not produce.
func testGendoc() {
	router := tanukirpc.NewRouter(
		struct{}{},
		tanukirpc.WithErrorBody[struct{}](buildErrorBody),
		tanukirpc.WithErrorHooker[struct{}](tanukirpc.DefaultErrorHooker()),
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
