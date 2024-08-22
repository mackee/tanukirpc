package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/cors"
	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/genclient"
)

//go:generate go run github.com/mackee/tanukirpc/cmd/gentypescript -out ./frontend/src/client.ts ./

type Status string

const (
	StatusTodo  Status = "todo"
	StatusDoing Status = "doing"
	StatusDone  Status = "done"
)

type Task struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type Registry struct {
	db map[string]*Task
}

func main() {
	reg := &Registry{db: map[string]*Task{}}
	router := tanukirpc.NewRouter(reg)
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"http://localhost:*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
	}))

	router.Get("/api/ping", tanukirpc.NewHandler(pingHandler))
	router.Get("/api/tasks", tanukirpc.NewHandler(tasksHandler))
	router.Post("/api/tasks", tanukirpc.NewHandler(addTaskHandler))

	tr := tanukirpc.NewTransformer(taskTransformer)
	tanukirpc.RouteWithTransformer(router, tr, "/api/tasks/{id}", func(r *tanukirpc.Router[*RegistryWithTask]) {
		r.Get("/", tanukirpc.NewHandler(getTaskHandler))
		r.Put("/", tanukirpc.NewHandler(changeTaskHandler))
		r.Delete("/", tanukirpc.NewHandler(removeTaskHandler))
	})

	address := "127.0.0.1:8080"
	log.Printf("Server started at %s", address)

	genclient.AnalyzeTarget(router)
	server := &http.Server{Addr: address, Handler: router}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sig

		log.Println("Server is shutting down...")
		if err := server.Shutdown(context.Background()); err != nil {
			log.Fatalf("Failed to shutdown server: %v", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Failed to start server: %v", err)
	}
}

type PingResponse struct {
	Message string `json:"message"`
}

func pingHandler(ctx tanukirpc.Context[*Registry], req struct{}) (*PingResponse, error) {
	return &PingResponse{Message: "pong"}, nil
}

type TasksResponse struct {
	Tasks []*Task `json:"tasks"`
}

func tasksHandler(ctx tanukirpc.Context[*Registry], req struct{}) (*TasksResponse, error) {
	tasks := make([]*Task, 0, len(ctx.Registry().db))
	for _, task := range ctx.Registry().db {
		tasks = append(tasks, task)
	}
	slices.SortFunc(tasks, func(a, b *Task) int {
		return cmp.Compare(b.ID, a.ID)
	})
	return &TasksResponse{Tasks: tasks}, nil
}

type TaskNewInput struct {
	Name        string `json:"name" form:"name" validate:"required"`
	Description string `json:"description" form:"description"`
}

type AddTaskRequest struct {
	Task TaskNewInput `json:"task" form:"task" validate:"required"`
}

type AddTaskResponse struct {
	Task *Task `json:"task" required:"true"`
}

func addTaskHandler(ctx tanukirpc.Context[*Registry], req AddTaskRequest) (*AddTaskResponse, error) {
	maxID := 0
	for id := range ctx.Registry().db {
		i, err := strconv.Atoi(id)
		if err != nil {
			return nil, fmt.Errorf("failed to convert id to int: %w", err)
		}
		if i > maxID {
			maxID = i
		}
	}
	task := &Task{
		ID:          strconv.Itoa(maxID + 1),
		Name:        req.Task.Name,
		Description: req.Task.Description,
		Status:      StatusTodo,
		CreatedAt:   time.Now(),
	}
	ctx.Registry().db[task.ID] = task

	return &AddTaskResponse{Task: task}, nil
}

type RegistryWithTask struct {
	*Registry
	task *Task
}

func taskTransformer(ctx tanukirpc.Context[*Registry]) (*RegistryWithTask, error) {
	id := tanukirpc.URLParam(ctx, "id")
	if id == "" {
		return nil, tanukirpc.WrapErrorWithStatus(http.StatusBadRequest, errors.New("id is required"))
	}
	task, ok := ctx.Registry().db[id]
	if !ok {
		return nil, tanukirpc.WrapErrorWithStatus(http.StatusNotFound, errors.New("task not found"))
	}
	return &RegistryWithTask{Registry: ctx.Registry(), task: task}, nil
}

type TaskResponse struct {
	Task *Task `json:"task" required:"true"`
}

func getTaskHandler(ctx tanukirpc.Context[*RegistryWithTask], _ struct{}) (*TaskResponse, error) {
	task := ctx.Registry().task
	return &TaskResponse{Task: task}, nil
}

type ChangeTaskRequest struct {
	Task struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Status      Status  `json:"status"`
	} `json:"task"`
}

func changeTaskHandler(ctx tanukirpc.Context[*RegistryWithTask], req ChangeTaskRequest) (*TaskResponse, error) {
	task := ctx.Registry().task

	if req.Task.Name != nil {
		task.Name = *req.Task.Name
	}
	if req.Task.Description != nil {
		task.Description = *req.Task.Description
	}
	task.Status = req.Task.Status
	ctx.Registry().db[task.ID] = task

	return &TaskResponse{Task: task}, nil
}

type RemoveTaskResponse struct {
	Status string `json:"status"`
}

func removeTaskHandler(ctx tanukirpc.Context[*RegistryWithTask], _ struct{}) (*RemoveTaskResponse, error) {
	delete(ctx.Registry().db, ctx.Registry().task.ID)
	return &RemoveTaskResponse{Status: "ok"}, nil
}
