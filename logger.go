package tanukirpc

import (
	gocontext "context"
	"fmt"
	"log/slog"

	"github.com/mackee/tanukirpc/internal/requestid"
)

var defaultLoggerKeys = []fmt.Stringer{requestid.RequestIDKey}

type loggerHandler struct {
	slog.Handler
	keys []fmt.Stringer
}

func (l *loggerHandler) Handle(ctx gocontext.Context, record slog.Record) error {
	for _, key := range l.keys {
		if v := ctx.Value(key); v != nil {
			record.AddAttrs(slog.Any(key.String(), v))
		}
	}
	return l.Handler.Handle(ctx, record)
}

// NewLogger returns a new logger with the given logger.
// This logger output with the informwation with request ID.
// If the given logger is nil, it returns use the default logger.
// keys is the whitelist of keys that use read from context.Context.
func NewLogger(logger *slog.Logger, keys []fmt.Stringer) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return slog.New(&loggerHandler{
		Handler: logger.Handler(),
		keys:    keys,
	})
}
