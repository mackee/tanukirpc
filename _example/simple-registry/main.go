package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/mackee/tanukirpc"
	_ "github.com/mattn/go-sqlite3"
)

type registry struct {
	db     *sql.DB
	logger *slog.Logger
}

type createAccountRequest struct {
	Name string `form:"name" validation:"required"`
}

type createAccountResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func createAccount(ctx tanukirpc.Context[*registry], req createAccountRequest) (*createAccountResponse, error) {
	db := ctx.Registry().db
	var id int
	if err := db.
		QueryRowContext(ctx, "INSERT INTO accounts (name) VALUES (?) RETURNING id", req.Name).
		Scan(&id); err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}
	ctx.Registry().logger.Info(
		"account created",
		slog.Group("account",
			slog.Int("id", id),
			slog.String("name", req.Name),
		),
	)

	return &createAccountResponse{
		ID:   id,
		Name: req.Name,
	}, nil
}

type accountRequest struct {
	ID int `urlparam:"id"`
}

type accountResponse struct {
	Name string `json:"name"`
}

func account(ctx tanukirpc.Context[*registry], req accountRequest) (*accountResponse, error) {
	db := ctx.Registry().db
	var name string
	if err := db.QueryRowContext(ctx, "SELECT name FROM accounts WHERE id = ?", req.ID).Scan(&name); err == sql.ErrNoRows {
		return nil, tanukirpc.WrapErrorWithStatus(http.StatusNotFound, err)
	} else if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	return &accountResponse{
		Name: name,
	}, nil
}

func main() {
	ctx := context.Background()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		logger.Error("failed to open database", slog.Any("error", err))
		return
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "CREATE TABLE accounts (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		logger.Error("failed to create table", slog.Any("error", err))
		return
	}

	// Registry scoped to the process
	// r := tanukirpc.NewRouter(&registry{db: db, logger: logger})
	// Registry scoped to the request
	r := tanukirpc.NewRouter(
		&registry{},
		tanukirpc.WithContextFactory(
			tanukirpc.NewContextHookFactory(
				func(w http.ResponseWriter, req *http.Request) (*registry, error) {
					reqID := middleware.GetReqID(req.Context())
					reqIDLogger := logger.With(slog.String("req_id", reqID))
					return &registry{db: db, logger: reqIDLogger}, nil
				},
			),
		),
	)
	// You can use chi middleware or `func (http.Handler) http.Handler`
	r.Use(middleware.RequestID)

	r.Post("/accounts", tanukirpc.NewHandler(createAccount))
	r.Get("/account/{id}", tanukirpc.NewHandler(account))

	if err := http.ListenAndServe(":8080", r); err != nil && err != http.ErrServerClosed {
		fmt.Println(err)
	}
}
