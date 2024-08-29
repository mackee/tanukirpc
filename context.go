package tanukirpc

import (
	gocontext "context"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
)

type Context[Reg any] interface {
	gocontext.Context
	Request() *http.Request
	Response() http.ResponseWriter
	Registry() Reg
	Defer(fn DeferFunc, priority ...DeferDoTiming)
	DeferDo(priority DeferDoTiming) error
}

type context[Reg any] struct {
	gocontext.Context
	req           *http.Request
	res           http.ResponseWriter
	registry      Reg
	deferStackMap deferStackMap
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

func (c *context[Reg]) Defer(fn DeferFunc, timings ...DeferDoTiming) {
	pc, file, line, ok := runtime.Caller(1)

	timing := DeferDoTimingAfterResponse
	if len(timings) > 0 {
		timing = timings[0]
	}
	c.deferStackMap.push(
		timing,
		&deferAction{
			fn: fn,
			caller: &DeferFuncCallerStack{
				PC:   pc,
				File: file,
				Line: line,
				Ok:   ok,
			},
		},
	)
}

func (c *context[Reg]) DeferDo(timing DeferDoTiming) error {
	return c.deferStackMap.do(timing)
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
	return NewContextHookFactory(func(w http.ResponseWriter, req *http.Request) (Reg2, error) {
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

type DeferFunc func() error

type deferAction struct {
	fn     DeferFunc
	caller *DeferFuncCallerStack
}

type DeferFuncError struct {
	Err    error
	Caller *DeferFuncCallerStack
}

type DeferFuncCallerStack struct {
	PC   uintptr
	File string
	Line int
	Ok   bool
}

func (d *DeferFuncCallerStack) String() string {
	if !d.Ok {
		return "unknown"
	}
	f := runtime.FuncForPC(d.PC)
	file := filepath.Base(d.File)

	return fmt.Sprintf("%s:%d %s", file, d.Line, f.Name())
}

func (d *DeferFuncError) Error() string {
	return fmt.Sprintf(
		"occured error in Deferred func that registered at %s: %s",
		d.Caller.String(),
		d.Err.Error(),
	)
}

func (d *DeferFuncError) Unwrap() error {
	return d.Err
}

func (d *deferAction) do() error {
	if err := d.fn(); err != nil {
		return &DeferFuncError{Err: err, Caller: d.caller}
	}
	return nil
}

type DeferDoTiming int

const (
	DeferDoTimingBeforeResponse DeferDoTiming = iota
	DeferDoTimingAfterResponse
)

type deferStackMap map[DeferDoTiming]*deferStack

func (d *deferStackMap) push(t DeferDoTiming, da *deferAction) {
	if *d == nil {
		*d = make(deferStackMap, 1)
	}

	if _, ok := (*d)[t]; !ok {
		ds := make(deferStack, 0, 1)
		(*d)[t] = &ds
	}
	(*d)[t].push(da)
}

func (d *deferStackMap) do(t DeferDoTiming) error {
	if *d == nil {
		return nil
	}
	if _, ok := (*d)[t]; !ok {
		return nil
	}
	return (*d)[t].do()
}

type deferStack []*deferAction

func (d *deferStack) push(da *deferAction) {
	if (*d) == nil {
		*d = make([]*deferAction, 0, 1)
	}
	*d = append(*d, da)
}

func (d *deferStack) pop() *deferAction {
	if len(*d) == 0 {
		return nil
	}
	da := (*d)[len(*d)-1]
	*d = (*d)[:len(*d)-1]
	return da
}

func (d *deferStack) do() error {
	if len(*d) == 0 {
		return nil
	}
	for {
		da := d.pop()
		if da == nil {
			break
		}
		if err := da.fn(); err != nil {
			return err
		}
	}
	return nil
}
