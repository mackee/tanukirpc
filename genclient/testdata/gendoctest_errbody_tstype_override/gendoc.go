package gendoctest_errbody_tstype_override

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

// Money has a custom MarshalJSON. The field below carries a tstype:"..."
// tag, so gentypescript respects the user-declared TypeScript shape and
// does NOT reject the error body.
type Money struct {
	Cents    int
	Currency string
}

func (m Money) MarshalJSON() ([]byte, error) {
	return []byte(`"` + m.Currency + `"`), nil
}

type ErrorBody struct {
	Message string `json:"message"`
	Amount  Money  `json:"amount" tstype:"string"`
}

func build(err error) ErrorBody {
	return ErrorBody{Message: err.Error()}
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
