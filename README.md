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
	"context"
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
- :o: defer hooks for cleanup (works correctly with Context Transformation)
- :o: Session management
- :o: Authentication flow
  - :o: OpenID Connect

### Registry injection and Context Transformation

Registry injection is a unique feature of `tanukirpc`. You can inject a registry object into the handler function via the `Context`.

A `ContextFactory` can generate a `Context` (and its associated `Registry`) for each request. For more details, please refer to [_example/simple-registry](./_example/simple-registry).

You can define a cleanup function for the registry created by the factory. The `NewContextHookFactory` function accepts an optional `closer` function (`func(ctx Context[Reg]) error`). This function is automatically registered using `ctx.Defer` and executed after the handler finishes, making it suitable for releasing resources associated with the per-request registry.

```go
// Example: Define a closer function when creating the factory
factory := tanukirpc.NewContextHookFactory(
    func(w http.ResponseWriter, req *http.Request) (*MyRegistry, error) {
        // ... create registry ...
        dbConn, err := connectToDB() // Example: Get a DB connection
        if err != nil {
            return nil, err
        }
        registry := &MyRegistry{db: dbConn}
        return registry, nil
    },
    func(ctx tanukirpc.Context[*MyRegistry]) error {
        // This function will be called after the handler
        registry := ctx.Registry()
        return registry.db.Close() // Example: Close the DB connection
    },
)
// Use WithContextFactory option
r := tanukirpc.NewRouter(struct{}{}, tanukirpc.WithContextFactory(factory))
```

Furthermore, `tanukirpc` allows composing contexts using `RouteWithTransformer`. This function takes an existing router (`*Router[Reg1]`), a `Transformer[Reg1, Reg2]`, a route pattern, and a function to define routes within the transformed context (`func(r *Router[Reg2])`). It also accepts optional `closer` functions (`func(ctx Context[Reg2]) error`) specific to the transformed context.

This enables creating nested routing structures where inner routes operate with a different registry (`Reg2`) derived from the outer registry (`Reg1`). The transformer defines how to get `Reg2` from `Context[Reg1]`. Importantly, `Defer` calls registered in the outer context (`Context[Reg1]`) are correctly executed even when the request is handled within the inner, transformed context (`Context[Reg2]`). The `closer` functions provided to `RouteWithTransformer` are executed after the inner handlers complete, allowing for resource cleanup specific to the transformed context.

```go
// Example: Using RouteWithTransformer
type OuterRegistry struct { /* ... */ }
type InnerRegistry struct { /* ... derived from OuterRegistry */ }

// Define how to transform OuterRegistry context to InnerRegistry
transformer := tanukirpc.NewTransformer(func(ctx tanukirpc.Context[*OuterRegistry]) (*InnerRegistry, error) {
    // ... logic to create InnerRegistry from ctx.Registry() ...
    innerReg := &InnerRegistry{ /* ... */ }
    // Register a defer function in the *outer* context if needed
    ctx.Defer(func() error { fmt.Println("Outer context defer"); return nil })
    return innerReg, nil
})

// Define a closer for the inner context
innerCloser := func(ctx tanukirpc.Context[*InnerRegistry]) error {
    fmt.Println("Closing inner context resources")
    // ... cleanup for InnerRegistry ...
    return nil
}

outerRouter := tanukirpc.NewRouter(&OuterRegistry{})

// Create nested routes with a transformed context
tanukirpc.RouteWithTransformer(outerRouter, transformer, "/inner", func(innerRouter *tanukirpc.Router[*InnerRegistry]) {
    innerRouter.Get("/data", tanukirpc.NewHandler(func(ctx tanukirpc.Context[*InnerRegistry], req struct{}) (*struct{}, error) {
        // Handler uses InnerRegistry via ctx.Registry()
        ctx.Defer(func() error { fmt.Println("Inner context defer"); return nil }) // Defer in inner context
        fmt.Println("Handling request with inner context")
        return &struct{}{}, nil
    }))
}, innerCloser) // Pass the closer for the inner context
```

#### Use case

* Database connection management (per-request or shared)
* Logger configuration per route group
* Authentication/Authorization context layering
* Resource binding by path parameter. Examples can be found in [_example/todo](./_example/todo).

### Request binding

`tanukirpc` supports the following request bindings by default:

* URL parameter (like a `/entity/{id}` path): use the `urlparam` struct tag
* Query String: use the `query` struct tag
* JSON (`application/json`): use the `json` struct tag
* Form (`application/x-www-form-urlencoded`): use the `form` struct tag
* Raw Body: use the `rawbody` struct tag with []byte or io.ReadCloser
  * also support naked []byte or io.ReadCloser

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

#### Response with Status Code

If you want to return a response with a specific status code, you can use the `tanukirpc.WrapErrorWithStatus`.

```go
// this handler returns a 404 status code
func notFoundHandler(ctx tanukirpc.Context[struct{}], struct{}) (*struct{}, error) {
    return nil, tanukirpc.WrapErrorWithStatus(http.StatusNotFound, errors.New("not found"))
}
```

Also, you can use the `tanukirpc.ErrorRedirectTo` function. This function returns a response with a 3xx status code and a `Location` header.

```go
// this handler returns a 301 status code
func redirectHandler(ctx tanukirpc.Context[struct{}], struct{}) (*struct{}, error) {
    return nil, tanukirpc.ErrorRedirectTo(http.StatusMovedPermanently, "/new-location")
}
```

#### Customizing the error response body

By default the error response body is `tanukirpc.ErrorMessage` (`{"error": {"message": string}}`). Branching on the free-form `message` string is fragile: any wording change on the server (refactor, i18n, additional context) silently breaks the client. `WithErrorBody` lets a project define its own error body type that is emitted on both the Go side and in the generated TypeScript client, so frontends can discriminate errors via typed fields (HTTP status, application code, ...).

##### Go-side API

```go
// A function the user supplies to build the inner error body from an error.
type ErrorBodyMarshaler[E any] func(err error) E

// Register the custom body type and how to build it.
//
//   - E is the Go struct that will be encoded as the inner error body.
//     Its json tags drive both the runtime encoding and the TypeScript
//     type emitted by gentypescript.
//   - The marshaler is invoked by the default ErrorHooker. It receives the
//     handler error (possibly wrapped via WrapErrorWithStatus) and returns
//     the value to encode.
//   - The router still resolves the HTTP status via ErrorWithStatus /
//     ErrorWithRedirect exactly as today; only the response body changes.
func WithErrorBody[Reg any, E any](marshaler ErrorBodyMarshaler[E]) RouterOption[Reg]
```

Usage:

```go
type ErrorBody struct {
    Message string `json:"message"`
    Status  int    `json:"status"`
    Code    string `json:"code,omitempty"`
}

func buildErrorBody(err error) ErrorBody {
    body := ErrorBody{Message: err.Error(), Status: http.StatusInternalServerError}
    var ews tanukirpc.ErrorWithStatus
    if errors.As(err, &ews) {
        body.Status = ews.Status()
    }
    var coded interface{ Code() string }
    if errors.As(err, &coded) {
        body.Code = coded.Code()
    }
    return body
}

// E is inferred from the marshaler; only Reg needs to be specified.
r := tanukirpc.NewRouter(
    registry,
    tanukirpc.WithErrorBody[*Registry](buildErrorBody),
)
```

The response body wire shape is `{"error": <E>}` (the same `error` envelope as the default `tanukirpc.ErrorMessage`, with the inner body replaced by your type).

Notes:

- When `WithErrorBody` is **not** used, the default `ErrorHooker` encodes `tanukirpc.ErrorMessage` exactly as before.
- `WithErrorBody` and `WithErrorHooker` install the hooker on the same slot. The later option wins, so pass `WithErrorHooker` after `WithErrorBody` if you need to fully take over error rendering.

##### gentypescript-side

`gentypescript` automatically picks up the type registered via `WithErrorBody` from the `tanukirpc.NewRouter` call that produced the analyzed router. No extra option is required.

Resulting `client.ts` shape:

```ts
export type ErrorResponse = {
  error: {
    message: string;
    status: number;
    code?: string;
  };
};

type apiSchemaCollection = {
  "GET /ping": {
    // ...
    Response: { /* ... */ } | ErrorResponse;
  };
};

export const isErrorResponse = (response: unknown): response is ErrorResponse => {
  const e = (response as { error?: unknown })?.error;
  return (
    typeof (e as { message?: unknown })?.message === "string" &&
    typeof (e as { status?: unknown })?.status === "number"
  );
};
```

The `isErrorResponse` predicate is derived from the registered struct: every primitive (string/number/boolean) json field that lacks `omitempty` becomes part of the narrowing check. Optional or non-primitive fields are ignored. When no custom body is registered, the existing `!!(response as { error: unknown })?.error` check is preserved.

##### Analyzer reach and limitations

The analyzer tracks the router value handed to `genclient.AnalyzeTarget` back to a `tanukirpc.NewRouter` call and classifies every `RouterOption` argument. The following patterns are recognized as long as the relevant code lives in the analyzed package:

- `tanukirpc.WithErrorBody[Reg](marshaler)` passed directly to `NewRouter`
- Router built by a same-package factory (`func newAppRouter() *tanukirpc.Router[Reg] { return tanukirpc.NewRouter(reg, tanukirpc.WithErrorBody[Reg](...)) }`)
- Options assembled by a helper and spread: `tanukirpc.NewRouter(reg, buildOptions()...)` (also when the helper is generic: `tanukirpc.NewRouter(reg, buildOptions[Reg]()...)`)
- Wrappers that return a `RouterOption[Reg]`: `func appErrorBody[Reg any]() tanukirpc.RouterOption[Reg] { return tanukirpc.WithErrorBody[Reg](...) }` passed to `NewRouter`
- Last-wins option order: a `WithErrorHooker` placed after `WithErrorBody` resets the generated client back to the default `{ error: { message: string } }` shape, matching `Router.apply` precedence

When an option cannot be statically classified — for example a `RouterOption` loaded from a package-level variable, chosen via `if/else` (`*ssa.Phi`), assembled with `append`, returned by a helper in **another package** the analyzer cannot follow, or a `...`-spread of any of those — `gentypescript` emits a diagnostic at the option site:

```
gentypescript: could not statically determine whether this RouterOption configures the error response body; the generated TypeScript ErrorResponse type may not match runtime behavior
```

The same warning is emitted when a same-package router factory, `RouterOption` wrapper, or `buildOptions(flag)...` spread helper has multiple return paths whose error-rendering choices disagree (e.g. one branch returns `WithErrorBody`, another returns the default or a `WithErrorHooker`). In that case the generated client still optimistically advertises the `ErrorResponse` type from the branch that has one — runtime may not match if the other branch fires — so the warning is your signal to reconcile the construction site.

`genclient.AnalyzeTarget` may be called at most once per analyzed package. A single generated `client.ts` can only represent one router's routes and one error body, so if the analyzer finds more than one `AnalyzeTarget` call it emits an error diagnostic at each call site and skips generation. Move the routers into separate packages (each with its own `go:generate` line and output path) when you need typed clients for both.

Further shapes of the registered error body type are validated separately.

The generator emits a **warning** when:

- The error body type is a pointer (`func build(err error) *ErrorBody`). `encoding/json` can emit `{"error": null}` for a nil pointer, but the generated `ErrorResponse` type is not nullable. Use a struct value (`func build(err error) ErrorBody`) instead.
- The error body struct has an exported field without a `json` tag. `encoding/json` would emit it under its Go name, but the generated TypeScript `ErrorResponse` only includes fields with explicit json tags. Add a tag (or rename) so the generated client matches the wire format.
- The error body struct has an **embedded** field. `encoding/json` flattens embedded struct fields into the parent object at runtime, but `gentypescript` does not represent them in the generated TypeScript `ErrorResponse`. Replace the embedding with explicit json-tagged fields so the generated client matches the wire format.
- The error body struct has an **unexported** field with a `json` tag (e.g. `message string \`json:"message"\``). `encoding/json` drops unexported fields at runtime regardless of the tag, so the generated TypeScript would advertise a field that never appears on the wire. Export the field (capitalize the name) so the tag actually takes effect.

The generator **refuses to generate** a client (emits an error diagnostic and produces no output) when:

- The error body struct has no json-tagged fields that the generator can render (an empty struct, a struct whose fields all lack json tags or use `json:"-"`, a struct that only contains embedded fields, or a struct whose only json-tagged fields are unexported). `encoding/json` would still emit `{"error":{}}` at runtime, but the resulting TypeScript `ErrorResponse` shape would be impossible for clients to satisfy. Add at least one exported, explicit json-tagged field.

`r.With(...)` and `r.Route(...)` chains preserve the parent router's error hooker at runtime, so passing the chained router to `AnalyzeTarget` still picks up `WithErrorBody` registered on the parent. Other router-typed values that the analyzer cannot trace back to a `NewRouter` call (a `*ssa.Phi` from an `if/else`, an `*ssa.Extract` from a multi-return helper, a function parameter, etc.) produce the same warning as unresolved options — the generated client falls back to the default error shape unless the construction is simplified.

**Per-route error bodies via `RouteWithTransformer` are not supported.** `WithErrorBody` is a `RouterOption[Reg]` and is only meant to be passed to `NewRouter`. Manually invoking it on the inner `*Router[Reg2]` inside a `RouteWithTransformer` callback — e.g. `tanukirpc.WithErrorBody[Reg2](build)(innerRouter)` — does change the inner router's hooker at runtime, but the generated TypeScript can only carry one project-wide `ErrorResponse`, so routes registered inside the transformer block will still be typed against the outer router's error body. Use a single project-wide error body via `WithErrorBody` on the top-level `NewRouter` call if you want gentypescript to track it.

If you see this warning and want a fully consistent typed `ErrorResponse`, pass `WithErrorBody[Reg](marshaler)` directly to `NewRouter` (or through a same-package wrapper/factory whose return paths all agree) so the analyzer can resolve it. Cases that remain unsupported by design include configuration-driven option lists (e.g. only appending `WithErrorBody` when a runtime flag is set) — those are inherently undecidable for static analysis, and the warning is the best the generator can offer.

##### Out of scope

- Per-endpoint error body types. `WithErrorBody` registers a single project-wide body type.
- Encoder selection per error type (always uses the router's `Codec`).
- Automatic Go-side error → status mapping via the body marshaler. Status is still resolved through `ErrorWithStatus`; the marshaler may reuse it but cannot set it.

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

`tanukirpc` supports defer hooks for cleanup. You can register a function using `ctx.Defer` to be called after the handler function has been executed. These hooks are executed in LIFO (Last-In, First-Out) order.

`Defer` supports three timings:
*   `DeferDoTimingAfterResponse` (default): Executes after the response has been written. Suitable for cleanup tasks like closing connections or logging.
*   `DeferDoTimingBeforeCheckError`: Executes before checking if the response is an error. This timing is useful for actions that should occur regardless of whether the response is an error, such as logging or cleanup tasks. It will run even if the handler returns an error using `tanukirpc.WrapErrorWithStatus`, unlike the other timings.
*   `DeferDoTimingBeforeResponse`: Executes before the response is written. Useful for modifying headers or performing actions just before sending the response.

Deferred functions work correctly even when using `RouteWithTransformer`. Functions deferred in an outer context will execute after functions deferred in an inner context (respecting LIFO order across context boundaries).

```go
func myHandler(ctx tanukirpc.Context[struct{}], req myRequest) (*myResponse, error) {
    // This will run after the response is sent (default)
    ctx.Defer(func() error {
        fmt.Println("Cleanup after response")
        // Close the database connection, release resources, logging, enqueue job etc...
        return nil
    })

    // This will run just before the response is sent
    ctx.Defer(func() error {
        fmt.Println("Action before response")
        ctx.Response().Header().Set("X-Custom-Header", "value")
        return nil
    }, tanukirpc.DeferDoTimingBeforeResponse)

    fmt.Println("Handler logic executing...")
    return &myResponse{Data: "Success"}, nil
}

fnc myErrorHandler(ctx tanukirpc.Context[struct{}], req myRequest) (*myResponse, error) {
    // This will run before checking if the response is an error
    ctx.Defer(func() error {
        fmt.Println("Cleanup before checking error")
        return nil
    }, tanukirpc.DeferDoTimingBeforeCheckError)

    // Simulate an error
    return nil, tanukirpc.WrapErrorWithStatus(http.StatusInternalServerError, errors.New("something went wrong"))
}
```

### Session Management

`tanukirpc` provides convenient utilities for session management. You can use the `gorilla/sessions` package or other session management libraries.

To get started, create a session store and wrap it using `tanukirpc/auth/gorilla.NewStore`.
```go
import (
    "github.com/gorilla/sessions"
    "github.com/mackee/tanukirpc/sessions/gorilla"
    tsessions "github.com/mackee/tanukirpc/sessions"
)

func newStore(secrets []byte) (tsessions.Store, error) {
    sessionStore := sessions.NewCookieStore(secrets)
    store, err := gorilla.NewStore(sessionStore)
    if err != nil {
        return nil, err
    }
    return store, nil
}
```

In `RegistryFactory`, you can create a session using the `tanukirpc/sessions.Store`.

```go
type RegistryFactory struct {
    Store tsessions.Store
}

type Registry struct {
    sessionAccessor tsessions.Accessor
}

func (r *RegistryFactory) NewRegistry(w http.ResponseWriter, req *http.Request) (*Registry, error) {
	accessor, err := r.Store.GetAccessor(req)
	if err != nil {
		if tsessions.IsInvalidSessionError(err) {
			// Decide how to recover in your application, for example:
			// - expire the broken cookie
			// - return tanukirpc.ErrorRedirectTo(...)
			// - return tanukirpc.WrapErrorWithStatus(...)
		}
		return nil, fmt.Errorf("failed to get session accessor: %w", err)
	}

    return &Registry{
        sessionAccessor: accessor,
    }, nil
}

func (r *Registry) Session() tsessions.Accessor {
    return r.sessionAccessor
}
```

The `Registry` type implements the `tanukirpc/sessions.RegistryWithAccessor` interface.

### Authentication Flow

`tanukirpc` supports the OpenID Connect authentication flow. You can use the `tanukirpc/auth/oidc.NewHandlers` function to create handlers for this flow, which includes a set of handlers to facilitate user authentication.

#### Requirements

`tanukirpc/auth/oidc.Handlers` requires a `Registry` that implements the `tanukirpc/sessions.RegistryWithAccessor` interface. For more details, refer to the [Session Management](#session-management) section.

#### Usage

```go
oidcAuth := oidc.NewHandlers(
    oauth2Config, // *golang.org/x/oauth2.Config
    provider,     // *github.com/coreos/go-oidc/v3/oidc.Provider
)
router.Route("/auth", func(router *tanukirpc.Router[*Registry]) {
    router.Get("/redirect", tanukirpc.NewHandler(oidcAuth.Redirect))
    router.Get("/callback", tanukirpc.NewHandler(oidcAuth.Callback))
    router.Get("/logout", tanukirpc.NewHandler(oidcAuth.Logout))
})
```

### Inertia.js support

`tanukirpc` provides an opt-in Inertia.js codec in the `github.com/mackee/tanukirpc/codec/inertiajs` package. It is not enabled by default, so API-only applications keep the default JSON/form/raw-body behavior.

Inertia handlers return a typed `inertiajs.Page[T]` response. The codec writes the initial HTML shell for normal browser requests and writes the Inertia page JSON when the request includes `X-Inertia: true`.

```go
import (
    "html/template"

    "github.com/mackee/tanukirpc"
    "github.com/mackee/tanukirpc/codec/inertiajs"
)

tmpl := template.Must(template.ParseFiles("templates/app.html"))
inertia := inertiajs.New(tmpl, inertiajs.WithAssetVersion("dev"))

router := tanukirpc.NewRouter(
    registry,
    tanukirpc.WithCodec[*Registry](tanukirpc.CodecList{
        inertia,
        tanukirpc.DefaultCodecList,
    }),
    tanukirpc.WithErrorHooker[*Registry](inertiajs.NewErrorHooker(inertia, nil)),
)

type HomeProps struct {
    Message string `json:"message"`
}

func homeHandler(ctx tanukirpc.Context[*Registry], req struct{}) (inertiajs.Page[HomeProps], error) {
    return inertiajs.Render("Home", HomeProps{Message: "Hello from Inertia.js"}), nil
}
```

See [_example/inertiajs](./_example/inertiajs) for a working Go + Inertia.js + React application.

## License

Copyright (c) 2024- [mackee](https://github.com/mackee)

Licensed under MIT License.
