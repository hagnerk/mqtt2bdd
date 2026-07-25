// Package logger provides structured logging with configurable levels
// using the standard library log/slog package.
package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Component names attached to every log entry via WithComponent. Exported as
// constants so the value cannot drift between call sites.
const (
	// ComponentMain identifies log entries emitted by the application entry point.
	ComponentMain = "main"
	// ComponentMQTT identifies log entries emitted by the MQTT client.
	ComponentMQTT = "mqtt"
	// ComponentDatabase identifies log entries emitted by the PostgreSQL client.
	ComponentDatabase = "database"
)

// NewLogger creates a slog.Logger writing to w at the specified level.
// Level is case-insensitive: "DEBUG", "INFO", "ERROR". Non-empty unrecognized
// values default to INFO and log a warning. Empty string silently defaults to INFO.
// Timestamps are emitted in UTC (RFC 3339 with a "Z" suffix).
func NewLogger(level string, w io.Writer) *slog.Logger {
	var logLevel slog.Level
	var unknown bool

	switch strings.ToUpper(level) {
	case "DEBUG":
		logLevel = slog.LevelDebug
	case "INFO":
		logLevel = slog.LevelInfo
	case "ERROR":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
		if level != "" {
			unknown = true
		}
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Restricted to the top-level record timestamp: a nested attribute a
			// caller happens to name "time" is left as the caller wrote it.
			if a.Key == slog.TimeKey && len(groups) == 0 {
				a.Value = slog.TimeValue(a.Value.Time().UTC())
			}
			return a
		},
	}
	handler := slog.NewTextHandler(w, opts)
	l := slog.New(handler)

	if unknown {
		// Tagged only for this warning; the returned logger stays untagged so each
		// caller attaches its own component.
		l.With("component", ComponentMain).Warn("unknown log level, defaulting to INFO", "event", "unknown_log_level", "level", level)
	}

	return l
}

// InitLogger initializes and returns a logger writing to stdout.
// It is the production entry point; tests should use NewLogger directly.
func InitLogger(level string) *slog.Logger {
	return NewLogger(level, os.Stdout)
}

// WithComponent returns a logger that tags every entry with the given component
// name. It is the single way a component tag is attached across the codebase.
func WithComponent(l *slog.Logger, component string) *slog.Logger {
	return l.With("component", component)
}
