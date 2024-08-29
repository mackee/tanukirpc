package tanukirpc

import (
	gocontext "context"
	"log/slog"
	"net/http"
	"time"
)

type AccessLogger interface {
	Log(ctx gocontext.Context, logger *slog.Logger, ww WrapResponseWriter, req *http.Request, err error, t1 time.Time, t2 time.Time) error
}

type WrapResponseWriter interface {
	http.ResponseWriter
	Status() int
	BytesWritten() int
}

type accessLogger struct{}

func (a *accessLogger) Log(ctx gocontext.Context, logger *slog.Logger, ww WrapResponseWriter, req *http.Request, err error, t1 time.Time, t2 time.Time) error {
	reqHostHeader := req.Header.Get("Host")
	reqContentType := req.Header.Get("Content-Type")
	respContentType := ww.Header().Get("Content-Type")

	logger.InfoContext(ctx, "accesslog",
		slog.String("host", reqHostHeader),
		slog.String("method", req.Method),
		slog.String("path", req.URL.String()),
		slog.String("proto", req.Proto),
		slog.String("remote", req.RemoteAddr),
		slog.String("request_content_type", reqContentType),
		slog.String("response_content_type", respContentType),
		slog.Int("status", ww.Status()),
		slog.Int("size", ww.BytesWritten()),
		slog.String("process_time", t2.Sub(t1).String()),
		slog.Time("start", t1),
		slog.Time("end", t2),
		slog.Bool("error", err != nil),
	)

	return nil
}
