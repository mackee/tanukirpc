package tanukirpc

import (
	gocontext "context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"
)

type listenAndServeConfig struct {
	disableTanukiupProxy bool
	shutdownTimeout      time.Duration
}

type ListenAndServeOption func(*listenAndServeConfig)

func WithDisableTanukiupProxy() ListenAndServeOption {
	return func(o *listenAndServeConfig) {
		o.disableTanukiupProxy = true
	}
}

func WithShutdownTimeout(d time.Duration) ListenAndServeOption {
	return func(o *listenAndServeConfig) {
		o.shutdownTimeout = d
	}
}

// ListenAndServe starts the server.
// If the context is canceled, the server will be shutdown.
func (r *Router[Reg]) ListenAndServe(ctx gocontext.Context, addr string, opts ...ListenAndServeOption) error {
	cfg := &listenAndServeConfig{}
	for _, o := range opts {
		o(cfg)
	}

	server := &http.Server{
		Addr:    addr,
		Handler: r,
	}
	go func() {
		<-ctx.Done()
		rctx, cancel := gocontext.WithTimeout(gocontext.Background(), 5*time.Second)
		defer cancel()

		slog.InfoContext(ctx, "Server is shutting down...")
		if err := server.Shutdown(rctx); err != nil {
			slog.ErrorContext(ctx, "failed to shutdown server", slog.Any("error", err))
		}
	}()
	var uds net.Listener
	if !cfg.disableTanukiupProxy {
		_uds, err := r.tanukiupUnixListener()
		if err != nil && !errors.Is(err, errTanukiupUDSNotFound) {
			return fmt.Errorf("failed to listen tanukiup unix domain socket: %w", err)
		}
		uds = _uds
	}

	if uds == nil {
		slog.InfoContext(ctx, "Server is starting...", slog.String("addr", addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("failed to listen and serve: %w", err)
		}
	} else {
		slog.InfoContext(ctx, "Server is starting with unix domain socket...")
		if err := server.Serve(uds); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("failed to serve: %w", err)
		}
	}
	return nil
}

var errTanukiupUDSNotFound = errors.New("tanukiup unix domain socket not found")

const (
	tanukiupUDSPathEnv = "TANUKIUP_UDS_PATH"
)

func (r *Router[Reg]) tanukiupUnixListener() (net.Listener, error) {
	p, ok := os.LookupEnv(tanukiupUDSPathEnv)
	if !ok {
		return nil, errTanukiupUDSNotFound
	}

	uds, err := net.Listen("unix", p)
	if err != nil {
		return nil, fmt.Errorf("failed to listen unix domain socket: %w", err)
	}
	return uds, nil
}
