package tanukirpc

import (
	"github.com/go-chi/chi/v5"
)

func URLParam[Reg any](ctx Context[Reg], name string) string {
	return chi.URLParam(ctx.Request(), name)
}
