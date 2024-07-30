# tanukirpc

`tanukirpc` is a practical, fast, type-safe, and easy-to-use RPC/Router library for Go. This library base on [`go-chi/chi`](https://github.com/go-chi/chi).


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

type HelloRequest struct {
    Name string `urlparam:"name"`
}

type HelloResponse struct {
    Message string `json:"message"`
}

func Hello(ctx *tanukirpc.Context[struct{}], req *HelloRequest) (*HelloResponse, error) {
    return &HelloResponse{
        Message: fmt.Sprintf("Hello, %s!", req.Name),
    }, nil
}

func main() {
    r := tanukirpc.NewRouter(struct{}{})
    r.GET("/hello/{name}", tanukirpc.NewHandler(Hello))

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

This is an example of database connection injection.

```go
package main

import (
    "context"
    "fmt"
    "net/http"
    "database/sql"

    "github.com/mackee/tanukirpc"
)

type AccountRequest struct {
    ID int `urlparam:"id"`
}

type AccountResponse struct {
    Name string `json:"name"`
}

type Registry struct {
    db *sql.DB
}

func Account(ctx *tanukirpc.Context[*Registry], req *AccountRequest) (*AccountResponse, error) {
    var name string
    if err := ctx.Registry().db.QueryRow("SELECT name FROM accounts WHERE id = ?", req.ID).Scan(&name); err == sql.ErrNoRows {
        return nil, tanukirpc.WrapErrorWithStatus(err, http.StatusNotFound)
    } else if err != nil {
        return nil, fmt.Errorf("failed to get account: %w", err)
    }

    return &AccountResponse{
        Name: name,
    }, nil
}

func main() {
    db, err := sql.Open(...)
    if err != nil {
        fmt.Println(err)
        return
    }
    defer db.Close()

    r := tanukirpc.NewRouter(&Registry{db: db})
    r.GET("/account/{id}", tanukirpc.NewHandler(Account))

    if err := http.ListenAndServe(":8080", r); err != nil && err != http.ErrServerClosed {
        fmt.Println(err)
    }
}
```

## License

Copyright (c) 2024- [mackee](https://github.com/mackee)

Licensed under MIT License.
