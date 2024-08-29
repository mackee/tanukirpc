package tanukirpc

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
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
		ww := middleware.NewWrapResponseWriter(w, req.ProtoMajor)
		t1 := time.Now()
		var t2 time.Time
		var lerr error
		defer func() {
			if t2.IsZero() {
				t2 = time.Now()
			}
			if err := r.accessLoggerLog(req.Context(), ww, req, lerr, t1, t2); err != nil {
				r.logger.ErrorContext(req.Context(), "access log error", slog.Any("error", err))
			}
		}()

		var reqBody Req
		if err := r.codec.Decode(req, &reqBody); err != nil {
			r.errorHooker.OnError(ww, req, r.codec, err)
			lerr = err
			return
		}
		if vreq, ok := canValidate(reqBody); ok {
			if err := vreq.Validate(); err != nil {
				ve := &ValidateError{err: err}
				r.errorHooker.OnError(ww, req, r.codec, ve)
				lerr = err
				return
			}
		}

		ctx, err := r.contextFactory.Build(ww, req)
		if err != nil {
			r.errorHooker.OnError(ww, req, r.codec, err)
			lerr = err
			return
		}

		res, err := h.h(ctx, reqBody)
		if err != nil {
			r.errorHooker.OnError(ww, req, r.codec, err)
			lerr = err
			return
		}

		if err := ctx.DeferDo(DeferDoTimingBeforeResponse); err != nil {
			r.errorHooker.OnError(ww, req, r.codec, err)
			lerr = err
			return
		}
		if err := r.codec.Encode(ww, req, res); err != nil {
			r.errorHooker.OnError(ww, req, r.codec, err)
			lerr = err
			return
		}
		t2 = time.Now()

		if err := ctx.DeferDo(DeferDoTimingAfterResponse); err != nil {
			r.logger.ErrorContext(ctx, "defer do error", slog.Any("error", err))
			lerr = err
			return
		}
	}
}
