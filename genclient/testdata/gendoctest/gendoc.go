package gendoctest

import (
	"time"

	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./client.ts ./

const (
	echoPath = "/echo"
)

func testGendoc() {
	router := newTestGendocRouter()
	type echoRequest struct {
		Message string `json:"message" validate:"required"`
	}
	type echoResponse struct {
		Message *string `json:"message"`
	}
	router.Get(
		echoPath,
		tanukirpc.NewHandler(
			func(ctx tanukirpc.Context[struct{}], req *echoRequest) (*echoResponse, error) {
				return &echoResponse{Message: &req.Message}, nil
			},
		),
	)
	router.Route("/nested", func(r *tanukirpc.Router[struct{}]) {
		type nowResponse struct {
			Now string `json:"now"`
		}
		r.Get("/now", tanukirpc.NewHandler(
			func(ctx tanukirpc.Context[struct{}], _ struct{}) (*nowResponse, error) {
				return &nowResponse{Now: time.Now().String()}, nil
			},
		))
		r.Get("/{epoch:[0-9]+}", tanukirpc.NewHandler(epochHandler))
	})

	genclient.AnalyzeTarget(router)
}

type exampleTestRouterRegistry struct {
	pingCounter int
}

func newTestGendocRouter() *tanukirpc.Router[struct{}] {
	router := tanukirpc.NewRouter(struct{}{})
	transformer := tanukirpc.NewTransformer(
		func(_ tanukirpc.Context[struct{}]) (*exampleTestRouterRegistry, error) {
			return &exampleTestRouterRegistry{}, nil
		},
	)

	type pingResponse struct {
		Message string `json:"message"`
	}
	type pingCounterResponse struct {
		Count int `json:"count"`
	}
	pingPostHandler := func(ctx tanukirpc.Context[*exampleTestRouterRegistry], _ struct{}) (*pingResponse, error) {
		ctx.Registry().pingCounter++
		return &pingResponse{"pong"}, nil
	}
	pingGetHandler := func(ctx tanukirpc.Context[*exampleTestRouterRegistry], _ struct{}) (*pingCounterResponse, error) {
		count := ctx.Registry().pingCounter
		return &pingCounterResponse{count}, nil
	}

	tanukirpc.RouteWithTransformer(router, transformer, "/ping", func(r *tanukirpc.Router[*exampleTestRouterRegistry]) {
		r.Post("/", tanukirpc.NewHandler(pingPostHandler))
		r.Get("/", tanukirpc.NewHandler(pingGetHandler))
		r.Route("/nested", func(r *tanukirpc.Router[*exampleTestRouterRegistry]) {
			r.Get("/", tanukirpc.NewHandler(pingGetHandler))
		})
	})

	return router
}

type epochRequest struct {
	Epoch int64 `urlparam:"epoch"`
}
type epochResponse struct {
	Datetime string `json:"datetime"`
}

func epochHandler(ctx tanukirpc.Context[struct{}], req *epochRequest) (*epochResponse, error) {
	return &epochResponse{Datetime: time.Unix(req.Epoch, 0).String()}, nil
}
