package main

import (
	"context"
	"fmt"

	"github.com/mackee/tanukirpc"
)

type helloRequest struct {
	Name string `urlparam:"name"`
}

type helloResponse struct {
	Message string `json:"message"`
}

func hello(ctx tanukirpc.Context[struct{}], req helloRequest) (*helloResponse, error) {
	return &helloResponse{
		Message: fmt.Sprintf("Hello, %s!", req.Name),
	}, nil
}

func main() {
	r := tanukirpc.NewRouter(struct{}{})
	r.Get("/hello/{name}", tanukirpc.NewHandler(hello))

	if err := r.ListenAndServe(context.Background(), ":8080"); err != nil {
		fmt.Println(err)
	}
}
