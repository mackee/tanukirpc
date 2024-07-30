package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mackee/tanukirpc"
)

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
	router.Get("/api/ping", tanukirpc.NewHandler(pingHandler))
	router.Post("/api/tasks", tanukirpc.NewHandler(addTaskHandler))

	tr := tanukirpc.NewTransformer(taskTransformer)
	tanukirpc.RouteWithTransformer(router, tr, "/api/tasks/{id}", func(r *tanukirpc.Router[*RegistryWithTask]) {
		r.Get("/", tanukirpc.NewHandler(getTaskHandler))
		r.Put("/", tanukirpc.NewHandler(changeTaskHandler))
		r.Delete("/", tanukirpc.NewHandler(removeTaskHandler))
	})

	address := "127.0.0.1:8080"
	log.Printf("Server started at %s", address)

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

type TaskNewInput struct {
	Name        string `json:"name" form:"name" validate:"required"`
	Description string `json:"description" form:"description"`
}

type AddTaskRequest struct {
	Task TaskNewInput `json:"task" form:"task"`
}

type AddTaskResponse struct {
	Task *Task `json:"task" form:"task"`
}

func addTaskHandler(ctx tanukirpc.Context[*Registry], req AddTaskRequest) (*AddTaskResponse, error) {
	task := &Task{
		ID:          "1",
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
	Task *Task `json:"task"`
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
