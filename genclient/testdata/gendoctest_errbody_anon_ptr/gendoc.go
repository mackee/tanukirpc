package gendoctest_errbody_anon_ptr

import (
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

// build returns a pointer to an anonymous struct. There is no named-type
// declaration position to attach the pointer warning to, so the analyzer must
// fall back to the AnalyzeTarget call site for the diagnostic instead of
// firing at token.NoPos (which would silently disappear in editors).
func build(err error) *struct {
	Message string `json:"message"`
} {
	return nil
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

	genclient.AnalyzeTarget(router) // want `gentypescript: error body type .* is a pointer`
}
