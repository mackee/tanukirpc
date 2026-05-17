package gendoctest_errbody_generic_spread

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

// buildOptions is parameterized on Reg. Calling buildOptions[Reg]() at the
// spread site lowers in SSA to a *ssa.Call whose result is wrapped in a
// *ssa.ChangeType so the parametric return type matches the concrete
// []RouterOption[struct{}] expected by NewRouter. collectOptionElementsRec
// must unwrap that conversion before the *ssa.Call branch fires, otherwise
// the WithErrorBody inside is treated as unresolved.
func buildOptions[Reg any]() []tanukirpc.RouterOption[Reg] {
	return []tanukirpc.RouterOption[Reg]{
		tanukirpc.WithErrorBody[Reg](buildErrorBody),
	}
}

func testGendoc() {
	router := tanukirpc.NewRouter(struct{}{}, buildOptions[struct{}]()...)
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
