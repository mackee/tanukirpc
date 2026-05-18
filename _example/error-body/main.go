package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go tool gentypescript -out ./frontend/src/client.ts ./

// ErrorBody is the application-defined error response shape. Its json tags
// drive both the runtime wire format and the TypeScript ErrorResponse type
// emitted by gentypescript.
type ErrorBody struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
	Code    string `json:"code,omitempty"`
}

// codedError adds a stable application code to an error. The marshaler picks
// it up so the client can branch on a fixed string instead of the free-form
// message.
type codedError struct {
	code string
	err  error
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Code() string  { return e.code }
func (e *codedError) Unwrap() error { return e.err }

func newCodedError(status int, code, msg string) error {
	return tanukirpc.WrapErrorWithStatus(status, &codedError{code: code, err: errors.New(msg)})
}

// buildErrorBody is the ErrorBodyMarshaler. Its return value becomes the
// entire response body (the marshaler controls the top-level wire shape),
// so the client sees `{"message":..., "status":..., "code":...}` directly.
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

type Registry struct {
	users map[string]*User
}

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func main() {
	reg := &Registry{
		users: map[string]*User{
			"1": {ID: "1", Name: "Alice"},
			"2": {ID: "2", Name: "Bob"},
		},
	}

	router := tanukirpc.NewRouter(
		reg,
		tanukirpc.WithErrorBody[*Registry](buildErrorBody),
	)

	router.Get("/api/users/{id}", tanukirpc.NewHandler(getUserHandler))

	genclient.AnalyzeTarget(router)

	address := "127.0.0.1:8080"
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()
	if err := router.ListenAndServe(ctx, address); err != nil {
		slog.ErrorContext(ctx, "Failed to start server", slog.Any("error", err))
	}
}

type GetUserRequest struct {
	ID string `urlparam:"id"`
}

type GetUserResponse struct {
	User *User `json:"user" required:"true"`
}

func getUserHandler(ctx tanukirpc.Context[*Registry], req GetUserRequest) (*GetUserResponse, error) {
	user, ok := ctx.Registry().users[req.ID]
	if !ok {
		return nil, newCodedError(http.StatusNotFound, "USER_NOT_FOUND", "user not found: id="+req.ID)
	}
	return &GetUserResponse{User: user}, nil
}
