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

// EffectiveLevel resolves a configured level name to the slog.Level actually in
// force. Level is case-insensitive: "DEBUG", "INFO", "ERROR". Empty and
// unrecognized values resolve to slog.LevelInfo, so the returned level is what
// the handler filters on regardless of what was requested.
func EffectiveLevel(level string) slog.Level {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewLogger creates a slog.Logger writing to w at the specified level.
// Level is case-insensitive: "DEBUG", "INFO", "ERROR". Non-empty unrecognized
// values default to INFO and log a warning. Empty string silently defaults to INFO.
// Timestamps are emitted in UTC (RFC 3339 with a "Z" suffix).
func NewLogger(level string, w io.Writer) *slog.Logger {
	logLevel := EffectiveLevel(level)
	// An unrecognized value resolves to INFO like an empty one, but only the
	// former is worth warning about: it means the operator asked for something.
	unknown := level != "" && !isKnownLevel(level)

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

// isKnownLevel reports whether level names one of the accepted levels. It is the
// only thing EffectiveLevel's default arm cannot express: that arm conflates
// "unset" with "misspelled", and only the latter deserves a warning.
func isKnownLevel(level string) bool {
	switch strings.ToUpper(level) {
	case "DEBUG", "INFO", "ERROR":
		return true
	default:
		return false
	}
}
