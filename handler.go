package tanukirpc

import (
	"log/slog"
	"net/http"
)

type Handler[Reg any] interface {
	build(r *Router[Reg]) http.HandlerFunc
}

func NewHandler[Req any, Res any, Reg any](h HandlerFunc[Req, Res, Reg]) Handler[Reg] {
	return &handler[Req, Res, Reg]{h: h}
}

type HandlerFunc[Req any, Res any, Reg any] func(Context[Reg], Req) (Res, error)

type handler[Req any, Res any, T any] struct {
	h HandlerFunc[Req, Res, T]
}

func (h *handler[Req, Res, Reg]) build(r *Router[Reg]) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var reqBody Req
		if err := r.codec.Decode(req, &reqBody); err != nil {
			r.errorHooker.OnError(w, req, r.codec, err)
			return
		}
		if vreq, ok := canValidate(reqBody); ok {
			if err := vreq.Validate(); err != nil {
				ve := &ValidateError{err: err}
				r.errorHooker.OnError(w, req, r.codec, ve)
				return
			}
		}

		ctx, err := r.contextFactory.Build(w, req)
		if err != nil {
			r.errorHooker.OnError(w, req, r.codec, err)
			return
		}

		res, err := h.h(ctx, reqBody)
		if err != nil {
			r.errorHooker.OnError(w, req, r.codec, err)
			return
		}

		if err := ctx.DeferDo(DeferDoTimingBeforeResponse); err != nil {
			r.errorHooker.OnError(w, req, r.codec, err)
			return
		}
		if err := r.codec.Encode(w, req, res); err != nil {
			r.errorHooker.OnError(w, req, r.codec, err)
			return
		}

		if err := ctx.DeferDo(DeferDoTimingAfterResponse); err != nil {
			r.logger.ErrorContext(ctx, "defer do error", slog.Any("error", err))
			return
		}
	}
}
