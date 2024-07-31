# tanukirpc

`tanukirpc` is a practical, fast-developing, type-safe, and easy-to-use RPC/Router library for Go. This library base on [`go-chi/chi`](https://github.com/go-chi/chi).

## Installation

```bash
go get -u github.com/mackee/tanukirpc
```

## Usage

This is a simple example of how to use `tanukirpc`.

```go
package main

import (
	"fmt"
	"net/http"

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

	if err := http.ListenAndServe(":8080", r); err != nil && err != http.ErrServerClosed {
		fmt.Println(err)
	}
}
```

## Features

- :o: Type-safe request/response handler
- :o: URL parameter, JSON, Form, or custom binding
- :o: Request validation by [go-playground/validator](https://github.com/go-playground/validator)
- :o: Custom error handling
- :o: Registry injection
  - for a Dependency Injection

### Registry injection

Registry injection is unique feature of `tanukirpc`. You can inject a registry object to the handler function.

Additionally, Registry can be generated for each request. For more details, please refer to [_example/simple-registry](./_example/simple-registry).

#### Use case

* Database connection
* Logger
* Authentication information
* Resource binding by path parameter. Examples can be found in [_example/todo](./_example/todo).

### Request binding

`tanukirpc` supports the following request bindings by default:

* URL parameter (like a `/entity/{id}` path): use the `urlparam` struct tag
* JSON (`application/json`): use the `json` struct tag
* Form (`application/x-www-form-urlencoded`): use the `form` struct tag

If you want to use other bindings, you can implement the `tanukirpc.Codec` interface and specify it using the `tanukirpc.WithCodec` option when initializing the router.

```go
tanukirpc.NewRouter(YourRegistry, tanukirpc.WithCodec(yourCodec))
```

### Request validation

`tanukirpc` automatically validation by `go-playground/validator` when contains `validate` struct tag in request struct.

```go
type YourRequest struct {
    Name string `form:"name" validate:"required"`
}
```

If you want to use custom validation, you can implement the `tanukirpc.Validatable` interface in your request struct. `tanukirpc` will call the `Validatable.Validate` method after binding the request and before calling the handler function.

### Error handling

`tanukirpc` has a default error handler. If you want to use custom error handling, you can implement the `tanukirpc.ErrorHooker` interface and use this with the `tanukirpc.WithErrorHooker` option when initializing the router.

### Middleware

You can use `tanukirpc` with [go-chi/chi/middleware](https://pkg.go.dev/github.com/go-chi/chi/v5@v5.1.0/middleware) or `func (http.Handler) http.Handler` style middlewares. [gorilla/handlers](https://pkg.go.dev/github.com/gorilla/handlers) is also included in this.

If you want to use middleware, you can use `*Router.Use` or `*Router.With`.

## License

Copyright (c) 2024- [mackee](https://github.com/mackee)

Licensed under MIT License.
