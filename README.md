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

	if err := r.ListenAndServe(context.Background(), ":8080"); err != nil {
		fmt.Println(err)
	}
}
```

## Features

- :o: Type-safe request/response handler
- :o: URL parameter, Query String, JSON, Form, or custom binding
- :o: Request validation by [go-playground/validator](https://github.com/go-playground/validator)
- :o: Custom error handling
- :o: Registry injection
  - for a Dependency Injection
- :o: A development server command that automatically restarts on file changes
  - use `tanukiup` command
- :o: Generate TypeScript client code
  - use `gentypescript` command
- :o: defer hooks for cleanup

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
* Query String: use the `query` struct tag
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

### `tanukiup` command

The `tanukiup` command is very useful during development. When you start your server via the `tanukiup` command, it detects file changes, triggers a build, and restarts the server.

### Usage
You can use the `tanukiup` command as follows:
```sh
$ go run github.com/mackee/tanukirpc/cmd/tanukiup -dir ./...
```

- The `-dir` option specifies the directory to be watched. By appending `...` to the end, it recursively includes all subdirectories in the watch scope. If you want to exclude certain directories, use the `-ignore-dir` option. You can specify multiple directories by providing comma-separated values or by using the option multiple times. By default, the server will restart when files with the `.go` extension are updated.

- The `-addr` option allows the `tanukiup` command to act as a server itself. After building and starting the server application created with `tanukirpc`, it proxies requests to this process. The application must be started with `*tanukirpc.Router.ListenAndServe`; otherwise, the `-addr` option will not function. Only the paths registered with `tanukirpc.Router` will be proxied to the server application.

- Additionally, there is an option called `-catchall-target` that can be used in conjunction with `-addr`. This option allows you to proxy requests for paths that are not registered with `tanukirpc.Router` to another server address. This is particularly useful when working with a frontend development server (e.g., webpack, vite).

Additionally, it detects the `go:generate` lines for the `gentypescript` command mentioned later, and automatically runs them before restarting.

### Client code generation

A web application server using `tanukirpc` can generate client-side code based on the type information of each endpoint.

`gentypescript` generates client-side code specifically for TypeScript. By using the generated client implementation, you can send and receive API requests with type safety for each endpoint.

To generate the client code, first call `genclient.AnalyzeTarget` with the router as an argument to clearly define the target router.

Next, add the following go:generate line:

```go
//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./frontend/src/client.ts ./
```

The `-out` option specifies the output file name. Additionally, append `./` to specify the package to be analyzed.

When you run `go generate ./` in the package containing this file, or when you start the server via the aforementioned `tanukiup` command, the TypeScript client code will be generated.

For more detailed usage, refer to the [_example/todo](./_example/todo) directory.

### Defer hooks

`tanukirpc` supports defer hooks for cleanup. You can register a function to be called after the handler function has been executed.

```go
func (ctx *tanukirpc.Context[struct{}], struct{}) (*struct{}, error) {
    ctx.Defer(func() error {
        // Close the database connection, release resources, logging, enqueue job etc...
    })
    return &struct{}{}, nil
}
```

## License

Copyright (c) 2024- [mackee](https://github.com/mackee)

Licensed under MIT License.
