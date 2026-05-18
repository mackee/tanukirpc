package gendoctest_errbody_time_whitelist

import (
	"time"

	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

// ErrorBody exercises every nested position where time.Time can appear. The
// whitelist (time.Time → "string") must apply consistently:
//   - direct field
//   - pointer field (renders optional)
//   - slice field (renders T[])
//   - map value
//   - slice as a map value (typeInfo recursing through a map value into a
//     slice element)
type ErrorBody struct {
	Message    string                `json:"message"`
	OccurredAt time.Time             `json:"occurred_at"`
	UpdatedAt  *time.Time            `json:"updated_at,omitempty"`
	Timeline   []time.Time           `json:"timeline"`
	Stamps     map[string]time.Time  `json:"stamps"`
	Series     map[string][]time.Time `json:"series"`
}

func build(err error) ErrorBody {
	return ErrorBody{Message: err.Error(), OccurredAt: time.Now()}
}

func testGendoc() {
	router := tanukirpc.NewRouter(
		struct{}{},
		tanukirpc.WithErrorBody[struct{}](build),
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
