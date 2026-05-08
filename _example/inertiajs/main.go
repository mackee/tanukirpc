package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"mime"
	"net/http"
	"os/signal"
	"slices"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/mackee/tanukirpc"
	"github.com/mackee/tanukirpc/codec/inertiajs"
)

//go:embed templates/app.html
var templateFS embed.FS

type TaskStatus string

const (
	StatusTodo  TaskStatus = "todo"
	StatusDoing TaskStatus = "doing"
	StatusDone  TaskStatus = "done"
)

type Task struct {
	ID        string
	Title     string
	Notes     string
	Status    TaskStatus
	CreatedAt time.Time
}

type TaskView struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Notes     string `json:"notes"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

type Registry struct {
	mu     sync.Mutex
	nextID int
	tasks  map[string]*Task
}

func NewRegistry() *Registry {
	reg := &Registry{
		nextID: 1,
		tasks:  map[string]*Task{},
	}
	reg.createTask("Read the protocol", "Start from the Inertia page object and headers.", StatusDone)
	reg.createTask("Return a typed Page", "Handlers return inertiajs.Render(component, props).", StatusDoing)
	reg.createTask("Try client navigation", "Use the links to move without full page reloads.", StatusTodo)
	return reg
}

func (r *Registry) createTask(title, notes string, status TaskStatus) *Task {
	id := strconv.Itoa(r.nextID)
	r.nextID++
	task := &Task{
		ID:        id,
		Title:     title,
		Notes:     notes,
		Status:    status,
		CreatedAt: time.Now().UTC(),
	}
	r.tasks[id] = task
	return task
}

func (r *Registry) addTask(title, notes string) *Task {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.createTask(title, notes, StatusTodo)
}

func (r *Registry) listTasks() []TaskView {
	r.mu.Lock()
	defer r.mu.Unlock()
	tasks := make([]TaskView, 0, len(r.tasks))
	for _, task := range r.tasks {
		tasks = append(tasks, toTaskView(task))
	}
	slices.SortFunc(tasks, func(a, b TaskView) int {
		return cmpTaskID(a.ID, b.ID)
	})
	return tasks
}

func (r *Registry) findTask(id string) (TaskView, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[id]
	if !ok {
		return TaskView{}, false
	}
	return toTaskView(task), true
}

func cmpTaskID(a, b string) int {
	ai, aerr := strconv.Atoi(a)
	bi, berr := strconv.Atoi(b)
	if aerr != nil || berr != nil {
		return 0
	}
	return ai - bi
}

func toTaskView(task *Task) TaskView {
	return TaskView{
		ID:        task.ID,
		Title:     task.Title,
		Notes:     task.Notes,
		Status:    string(task.Status),
		CreatedAt: task.CreatedAt.Format(time.RFC3339),
	}
}

func main() {
	router, err := NewRouter(NewRegistry())
	if err != nil {
		slog.Error("failed to build router", slog.Any("error", err))
		return
	}

	address := "127.0.0.1:8080"
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()
	slog.InfoContext(ctx, "starting Inertia.js example", slog.String("address", address))
	if err := router.ListenAndServe(ctx, address); err != nil {
		slog.ErrorContext(ctx, "failed to start server", slog.Any("error", err))
	}
}

func NewRouter(reg *Registry) (*tanukirpc.Router[*Registry], error) {
	tmpl, err := template.ParseFS(templateFS, "templates/app.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse inertia template: %w", err)
	}
	inertia := inertiajs.New(tmpl, inertiajs.WithAssetVersion("dev"))
	router := tanukirpc.NewRouter(
		reg,
		tanukirpc.WithCodec[*Registry](tanukirpc.CodecList{
			inertia,
			tanukirpc.DefaultCodecList,
		}),
		tanukirpc.WithErrorHooker[*Registry](inertiajs.NewErrorHooker(inertia, inertiaErrorPage)),
	)
	router.Use(normalizeContentType)
	router.Get("/", tanukirpc.NewHandler(homeHandler))
	router.Get("/tasks", tanukirpc.NewHandler(tasksIndexHandler))
	router.Post("/tasks", tanukirpc.NewHandler(createTaskHandler))
	router.Get("/tasks/{id}", tanukirpc.NewHandler(taskShowHandler))
	return router, nil
}

func normalizeContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		contentType := req.Header.Get("Content-Type")
		if contentType != "" {
			mediaType, _, err := mime.ParseMediaType(contentType)
			if err == nil {
				req.Header.Set("Content-Type", mediaType)
			}
		}
		next.ServeHTTP(w, req)
	})
}

type HomeProps struct {
	ProjectName string `json:"projectName"`
	TaskCount   int    `json:"taskCount"`
}

func homeHandler(ctx tanukirpc.Context[*Registry], _ struct{}) (inertiajs.Page[HomeProps], error) {
	return inertiajs.Render("Home", HomeProps{
		ProjectName: "tanukirpc + Inertia.js",
		TaskCount:   len(ctx.Registry().listTasks()),
	}), nil
}

type TasksIndexProps struct {
	Tasks []TaskView `json:"tasks"`
}

func tasksIndexHandler(ctx tanukirpc.Context[*Registry], _ struct{}) (inertiajs.Page[TasksIndexProps], error) {
	return inertiajs.Render("Tasks/Index", TasksIndexProps{
		Tasks: ctx.Registry().listTasks(),
	}), nil
}

type TaskShowRequest struct {
	ID string `urlparam:"id"`
}

type TaskShowProps struct {
	Task TaskView `json:"task"`
}

func taskShowHandler(ctx tanukirpc.Context[*Registry], req TaskShowRequest) (inertiajs.Page[TaskShowProps], error) {
	task, ok := ctx.Registry().findTask(req.ID)
	if !ok {
		return inertiajs.Page[TaskShowProps]{}, tanukirpc.WrapErrorWithStatus(http.StatusNotFound, fmt.Errorf("task %s not found", req.ID))
	}
	return inertiajs.Render("Tasks/Show", TaskShowProps{Task: task}), nil
}

type CreateTaskRequest struct {
	Title string `json:"title" form:"title" validate:"required"`
	Notes string `json:"notes" form:"notes"`
}

type CreateTaskResponse struct{}

func createTaskHandler(ctx tanukirpc.Context[*Registry], req CreateTaskRequest) (*CreateTaskResponse, error) {
	if req.Title == "" {
		return nil, tanukirpc.WrapErrorWithStatus(http.StatusBadRequest, errors.New("title is required"))
	}
	ctx.Registry().addTask(req.Title, req.Notes)
	return nil, tanukirpc.ErrorRedirectTo(http.StatusSeeOther, "/tasks")
}

func inertiaErrorPage(_ *http.Request, err error, status int) inertiajs.Page[map[string]any] {
	return inertiajs.Render("Error", map[string]any{
		"message": err.Error(),
		"status":  status,
	})
}
