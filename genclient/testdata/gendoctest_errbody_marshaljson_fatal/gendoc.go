package gendoctest_errbody_marshaljson_fatal

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

// Money has a custom MarshalJSON that emits a string. gentypescript cannot
// know that statically and rejects the error body until the field is given
// a tstype:"..." tag.
type Money struct {
	Cents    int
	Currency string
}

func (m Money) MarshalJSON() ([]byte, error) {
	return []byte(`"` + m.Currency + `"`), nil
}

type ErrorBody struct {
	Message string `json:"message"`
	Amount  Money  `json:"amount"` // want `gentypescript: error body field has type .*\.Money with a custom MarshalJSON`
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
