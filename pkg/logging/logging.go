// Package logging initialises a structured slog.Logger for the Distill server.
// It supports JSON (production) and text (human-readable) output formats and
// four log levels: debug, info, warn, error.
//
// Usage:
//
//	logger := logging.New(logging.Config{Level: "info", Format: "json"})
//	logger.Info("request completed", "path", "/v1/dedupe", "latency_ms", 14)
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Format selects the log output format.
type Format string

const (
	FormatJSON Format = "json"
	FormatText Format = "text"
)

// Config controls logger initialisation.
type Config struct {
	// Level is one of "debug", "info", "warn", "error". Default: "info".
	Level string

	// Format is "json" or "text". Default: "json".
	Format Format

	// Output is the writer to log to. Default: os.Stderr.
	Output io.Writer

	// AddSource adds the source file and line to every log record.
	AddSource bool
}

// New creates a configured *slog.Logger. It does not replace the default
// slog logger — callers should store and pass the returned logger explicitly.
func New(cfg Config) *slog.Logger {
	if cfg.Output == nil {
		cfg.Output = os.Stderr
	}

	level := parseLevel(cfg.Level)

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.AddSource,
	}

	var handler slog.Handler
	if cfg.Format == FormatText {
		handler = slog.NewTextHandler(cfg.Output, opts)
	} else {
		handler = slog.NewJSONHandler(cfg.Output, opts)
	}

	return slog.New(handler)
}

// NewDefault returns a JSON logger at info level writing to stderr.
// Equivalent to New(Config{}).
func NewDefault() *slog.Logger {
	return New(Config{})
}

// NewDebug returns a text logger at debug level — useful for local development.
func NewDebug() *slog.Logger {
	return New(Config{Level: "debug", Format: FormatText})
}

// parseLevel converts a string level to slog.Level. Unknown values default to Info.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
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

// WithRequestID returns a child logger with a request_id attribute attached.
func WithRequestID(logger *slog.Logger, requestID string) *slog.Logger {
	return logger.With("request_id", requestID)
}

// WithTraceID returns a child logger with a trace_id attribute attached.
func WithTraceID(logger *slog.Logger, traceID string) *slog.Logger {
	return logger.With("trace_id", traceID)
}

// WithComponent returns a child logger scoped to a named component.
func WithComponent(logger *slog.Logger, component string) *slog.Logger {
	return logger.With("component", component)
}
