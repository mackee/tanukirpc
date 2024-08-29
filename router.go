package tanukirpc

import (
	gocontext "context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mackee/tanukirpc/internal/requestid"
)

var defaultMiddleware = []func(http.Handler) http.Handler{
	requestid.Middleware,
	middleware.RealIP,
	middleware.Recoverer,
}

type Router[Reg any] struct {
	cr                chi.Router
	codec             Codec
	contextFactory    ContextFactory[Reg]
	logger            *slog.Logger
	errorHooker       ErrorHooker
	accessLogger      AccessLogger
	defaultMiddleware []func(http.Handler) http.Handler
}

func NewRouterWithNoRegistry[Reg struct{}](opts ...RouterOption[Reg]) *Router[Reg] {
	router := &Router[Reg]{
		cr:                chi.NewRouter(),
		codec:             DefaultCodecList,
		contextFactory:    &DefaultContextFactory[Reg]{registry: struct{}{}},
		errorHooker:       &errorHooker{},
		logger:            NewLogger(slog.Default(), defaultLoggerKeys),
		accessLogger:      &accessLogger{},
		defaultMiddleware: defaultMiddleware,
	}
	router.apply(opts...)
	router.Use(router.defaultMiddleware...)

	return router
}

func NewRouter[Reg any](reg Reg, opts ...RouterOption[Reg]) *Router[Reg] {
	router := &Router[Reg]{
		cr:                chi.NewRouter(),
		codec:             DefaultCodecList,
		contextFactory:    &DefaultContextFactory[Reg]{registry: reg},
		errorHooker:       &errorHooker{},
		logger:            NewLogger(slog.Default(), defaultLoggerKeys),
		accessLogger:      &accessLogger{},
		defaultMiddleware: defaultMiddleware,
	}
	router.apply(opts...)
	router.Use(router.defaultMiddleware...)

	return router
}

func (r *Router[Reg]) apply(opts ...RouterOption[Reg]) *Router[Reg] {
	for _, opt := range opts {
		r = opt(r)
	}
	return r
}

func (r *Router[Reg]) Use(middlewares ...func(http.Handler) http.Handler) {
	r.cr.Use(middlewares...)
}

func (r *Router[Reg]) clone() *Router[Reg] {
	return &Router[Reg]{
		cr:             r.cr,
		codec:          r.codec,
		contextFactory: r.contextFactory,
		errorHooker:    r.errorHooker,
		logger:         r.logger,
		accessLogger:   r.accessLogger,
	}
}

func (r *Router[Reg]) cloneWithChiRouter(cr chi.Router) *Router[Reg] {
	r2 := r.clone()
	r2.cr = cr
	return r2
}

func (r *Router[Reg]) With(middlewares ...func(http.Handler) http.Handler) *Router[Reg] {
	return r.cloneWithChiRouter(r.cr.With(middlewares...))
}

func (r *Router[Reg]) Route(pattern string, fn func(r *Router[Reg])) *Router[Reg] {
	return r.cloneWithChiRouter(r.cr.Route(pattern, func(cr chi.Router) {
		fn(r.cloneWithChiRouter(cr))
	}))
}

func (r *Router[Reg]) Mount(pattern string, h http.Handler) {
	r.cr.Mount(pattern, h)
}

func (r *Router[Reg]) Connect(pattern string, h Handler[Reg]) {
	r.cr.Connect(pattern, h.build(r))
}

func (r *Router[Reg]) Delete(pattern string, h Handler[Reg]) {
	r.cr.Delete(pattern, h.build(r))
}

func (r *Router[Reg]) Get(pattern string, h Handler[Reg]) {
	r.cr.Get(pattern, h.build(r))
}

func (r *Router[Reg]) Head(pattern string, h Handler[Reg]) {
	r.cr.Head(pattern, h.build(r))
}

func (r *Router[Reg]) Options(pattern string, h Handler[Reg]) {
	r.cr.Options(pattern, h.build(r))
}

func (r *Router[Reg]) Patch(pattern string, h Handler[Reg]) {
	r.cr.Patch(pattern, h.build(r))
}

func (r *Router[Reg]) Post(pattern string, h Handler[Reg]) {
	r.cr.Post(pattern, h.build(r))
}

func (r *Router[Reg]) Put(pattern string, h Handler[Reg]) {
	r.cr.Put(pattern, h.build(r))
}

func (r *Router[Reg]) Trace(pattern string, h Handler[Reg]) {
	r.cr.Trace(pattern, h.build(r))
}

func (r *Router[Reg]) NotFound(h Handler[Reg]) {
	r.cr.NotFound(h.build(r))
}

func (r *Router[Reg]) MethodNotAllowed(h Handler[Reg]) {
	r.cr.MethodNotAllowed(h.build(r))
}

func (r *Router[Reg]) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.cr.ServeHTTP(w, req)
}

func RouteWithTransformer[Reg1 any, Reg2 any](r *Router[Reg1], tr Transformer[Reg1, Reg2], pattern string, fn func(r *Router[Reg2])) *Router[Reg1] {
	return r.Route(pattern, func(r *Router[Reg1]) {
		cf := compositionContextHooker(r.contextFactory, tr)
		r2 := &Router[Reg2]{
			cr:             r.cr,
			codec:          r.codec,
			contextFactory: cf,
			errorHooker:    r.errorHooker,
			logger:         r.logger,
			accessLogger:   r.accessLogger,
		}
		fn(r2)
	})
}

func (r *Router[Reg]) accessLoggerLog(ctx gocontext.Context, w WrapResponseWriter, req *http.Request, err error, t1, t2 time.Time) error {
	if r.accessLogger == nil {
		return nil
	}
	return r.accessLogger.Log(ctx, r.logger, w, req, err, t1, t2)
}

type RouterOption[Reg any] func(*Router[Reg]) *Router[Reg]

func WithChiRouter[Reg any](cr chi.Router) RouterOption[Reg] {
	return func(r *Router[Reg]) *Router[Reg] {
		r.cr = cr
		return r
	}
}

func WithCodec[Reg any](codec Codec) RouterOption[Reg] {
	return func(r *Router[Reg]) *Router[Reg] {
		r.codec = codec
		return r
	}
}

func WithContextFactory[Reg any](cf ContextFactory[Reg]) RouterOption[Reg] {
	return func(r *Router[Reg]) *Router[Reg] {
		r.contextFactory = cf
		return r
	}
}

func WithErrorHooker[Reg any](eh ErrorHooker) RouterOption[Reg] {
	return func(r *Router[Reg]) *Router[Reg] {
		r.errorHooker = eh
		return r
	}
}

func WithLogger[Reg any](logger *slog.Logger) RouterOption[Reg] {
	return func(r *Router[Reg]) *Router[Reg] {
		r.logger = logger
		return r
	}
}

func WithAccessLogger[Reg any](al AccessLogger) RouterOption[Reg] {
	return func(r *Router[Reg]) *Router[Reg] {
		r.accessLogger = al
		return r
	}
}

func WithDefaultMiddleware[Reg any](middlewares ...func(http.Handler) http.Handler) RouterOption[Reg] {
	return func(r *Router[Reg]) *Router[Reg] {
		r.defaultMiddleware = middlewares
		return r
	}
}
