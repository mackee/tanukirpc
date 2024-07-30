package tanukirpc

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Router[Reg any] struct {
	cr             chi.Router
	codec          Codec
	contextFactory ContextFactory[Reg]
	errorHooker    ErrorHooker
}

func NewRouterWithNoRegistry[Reg struct{}](opts ...RouterOption[Reg]) *Router[Reg] {
	router := &Router[Reg]{
		cr:             chi.NewRouter(),
		codec:          DefaultCodecList,
		contextFactory: &DefaultContextFactory[Reg]{registry: struct{}{}},
		errorHooker:    &errorHooker{},
	}
	router.apply(opts...)
	return router
}

func NewRouter[Reg any](reg Reg, opts ...RouterOption[Reg]) *Router[Reg] {
	router := &Router[Reg]{
		cr:             chi.NewRouter(),
		codec:          DefaultCodecList,
		contextFactory: &DefaultContextFactory[Reg]{registry: reg},
		errorHooker:    &errorHooker{},
	}
	router.apply(opts...)
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
		}
		fn(r2)
	})
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
