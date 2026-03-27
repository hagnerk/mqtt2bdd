// Package logger provides structured logging with configurable levels
// using the standard library log/slog package.
package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// NewLogger creates a slog.Logger writing to w at the specified level.
// Level is case-insensitive: "DEBUG", "INFO", "ERROR". Non-empty unrecognized
// values default to INFO and log a warning. Empty string silently defaults to INFO.
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

	handler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: logLevel})
	l := slog.New(handler)

	if unknown {
		l.Warn("unknown log level, defaulting to INFO", "level", level)
	}

	return l
}

// InitLogger initializes and returns a logger writing to stdout.
// It is the production entry point; tests should use NewLogger directly.
func InitLogger(level string) *slog.Logger {
	return NewLogger(level, os.Stdout)
}
