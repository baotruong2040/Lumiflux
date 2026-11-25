package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

type ctxKey struct{}

// ParseLevel converts a level string into a slog.Level, defaulting to LevelInfo.
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// New creates a JSON structured slog.Logger writing to stdout.
func New(level, env string) *slog.Logger {
	l := NewWithWriter(os.Stdout, level)
	if strings.TrimSpace(env) != "" {
		l = l.With("env", env)
	}
	return l
}

// NewWithWriter creates a JSON structured slog.Logger writing to w.
func NewWithWriter(w io.Writer, level string) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}
	opts := &slog.HandlerOptions{
		Level: ParseLevel(level),
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}

// IntoContext attaches a logger to ctx.
func IntoContext(ctx context.Context, l *slog.Logger) context.Context {
	if l == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext extracts the logger from ctx, falling back to slog.Default().
func FromContext(ctx context.Context) *slog.Logger {
	if ctx != nil {
		if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
			return l
		}
	}
	return slog.Default()
}
