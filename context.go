package tanukirpc

import (
	gocontext "context"
	"net/http"
)

type Context[Reg any] interface {
	gocontext.Context
	Request() *http.Request
	Response() http.ResponseWriter
	Registry() Reg
}

type context[Reg any] struct {
	gocontext.Context
	req      *http.Request
	res      http.ResponseWriter
	registry Reg
}

type ContextFactory[Reg any] interface {
	Build(w http.ResponseWriter, req *http.Request) (Context[Reg], error)
}

type DefaultContextFactory[Reg any] struct {
	registry Reg
}

func (d *DefaultContextFactory[Reg]) Build(w http.ResponseWriter, req *http.Request) (Context[Reg], error) {
	return &context[Reg]{
		Context:  req.Context(),
		req:      req,
		res:      w,
		registry: d.registry,
	}, nil
}

func (c *context[Reg]) Request() *http.Request {
	return c.req
}

func (c *context[Reg]) Response() http.ResponseWriter {
	return c.res
}

func (c *context[Reg]) Registry() Reg {
	return c.registry
}

type contextHookFactory[Reg any] struct {
	fn func(w http.ResponseWriter, req *http.Request) (Reg, error)
}

func NewContextHookFactory[Reg any](fn func(w http.ResponseWriter, req *http.Request) (Reg, error)) ContextFactory[Reg] {
	return &contextHookFactory[Reg]{fn: fn}
}

func (c *contextHookFactory[Reg]) Build(w http.ResponseWriter, req *http.Request) (Context[Reg], error) {
	registry, err := c.fn(w, req)
	if err != nil {
		return nil, err
	}

	return &context[Reg]{
		Context:  req.Context(),
		req:      req,
		res:      w,
		registry: registry,
	}, nil
}

type Transformer[Reg1 any, Reg2 any] interface {
	Transform(ctx Context[Reg1]) (Reg2, error)
}

func NewTransformer[Reg1 any, Reg2 any](fn func(ctx Context[Reg1]) (Reg2, error)) Transformer[Reg1, Reg2] {
	return &transformer[Reg1, Reg2]{fn: fn}
}

type transformer[Reg1 any, Reg2 any] struct {
	fn func(ctx Context[Reg1]) (Reg2, error)
}

func (t *transformer[Reg1, Reg2]) Transform(ctx Context[Reg1]) (Reg2, error) {
	return t.fn(ctx)
}

func compositionContextHooker[Reg1 any, Reg2 any](factory ContextFactory[Reg1], transformer Transformer[Reg1, Reg2]) ContextFactory[Reg2] {
	return NewContextHookFactory[Reg2](func(w http.ResponseWriter, req *http.Request) (Reg2, error) {
		var zeroReg2 Reg2
		ctx1, err := factory.Build(w, req)
		if err != nil {
			return zeroReg2, err
		}
		reg2, err := transformer.Transform(ctx1)
		if err != nil {
			return zeroReg2, err
		}
		return reg2, nil
	})
}
