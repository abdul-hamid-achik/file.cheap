package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

type contextKey string

const loggerKey contextKey = "logger"

var defaultLogger *slog.Logger

func Init(level string) {
	opts := &slog.HandlerOptions{
		Level: parseLevel(level),
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func Default() *slog.Logger {
	if defaultLogger == nil {
		Init("info")
	}
	return defaultLogger
}

func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return l
	}
	return Default()
}

func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

func NewTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
