package gendoctest_errbody_hooker_value_with_ptr_marker

import (
	"log/slog"
	"net/http"

	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

// TypedBody is the body shape that the marker method below claims.
type TypedBody struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
}

// ValueHooker has OnError on the value receiver (so a value satisfies the
// ErrorHooker interface) but ErrorBodyType only on the pointer receiver. A
// value passed to WithErrorHooker therefore does NOT satisfy
// tanukirpc.ErrorHookerWithBody[TypedBody] at runtime — Go's method set
// rules exclude pointer-receiver methods from values.
//
// gentypescript must mirror that and fall back to the default error shape
// for this construction. A previous bug used addressable=true in the marker
// lookup and produced a typed `ErrorResponse = TypedBody` instead.
type ValueHooker struct{}

func (h ValueHooker) OnError(w http.ResponseWriter, req *http.Request, logger *slog.Logger, codec tanukirpc.Codec, err error) {
	_ = w
	_ = req
	_ = logger
	_ = codec
	_ = err
}

func (h *ValueHooker) ErrorBodyType() TypedBody {
	var zero TypedBody
	return zero
}

func testGendoc() {
	router := tanukirpc.NewRouter(
		struct{}{},
		// Value (not pointer) — pointer-receiver marker is NOT in the
		// method set, so the typed body must not surface in client.ts.
		tanukirpc.WithErrorHooker[struct{}](ValueHooker{}),
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
