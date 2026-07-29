package logger_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/hagnerk/mqtt2bdd/internal/logger"
)

// fieldValue extracts the value of a key=value field from a slog TextHandler line.
// It returns an empty string when the key is absent.
func fieldValue(line, key string) string {
	for _, field := range strings.Fields(line) {
		if rest, found := strings.CutPrefix(field, key+"="); found {
			return rest
		}
	}
	return ""
}

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name       string
		level      string
		debugShown bool
		infoShown  bool
		errorShown bool
	}{
		{"debug level", "DEBUG", true, true, true},
		{"info level", "INFO", false, true, true},
		{"error level", "ERROR", false, false, true},
		{"case insensitive debug", "debug", true, true, true},
		{"case insensitive info", "info", false, true, true},
		{"case insensitive error", "error", false, false, true},
		{"invalid level defaults to INFO", "INVALID", false, true, true},
		{"empty level defaults to INFO silently", "", false, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			l := logger.NewLogger(tt.level, &buf)
			if l == nil {
				t.Fatal("NewLogger() returned nil")
			}

			l.Debug("debug-probe")
			l.Info("info-probe")
			l.Error("error-probe")

			gotDebug := strings.Contains(buf.String(), "debug-probe")
			gotInfo := strings.Contains(buf.String(), "info-probe")
			gotError := strings.Contains(buf.String(), "error-probe")

			if gotDebug != tt.debugShown {
				t.Errorf("debug visible=%v, want %v (level=%q, output=%q)", gotDebug, tt.debugShown, tt.level, buf.String())
			}
			if gotInfo != tt.infoShown {
				t.Errorf("info visible=%v, want %v (level=%q, output=%q)", gotInfo, tt.infoShown, tt.level, buf.String())
			}
			if gotError != tt.errorShown {
				t.Errorf("error visible=%v, want %v (level=%q, output=%q)", gotError, tt.errorShown, tt.level, buf.String())
			}
		})
	}
}

// TestNewLogger_UnknownLevelWarning closes the one real coverage gap in this file:
// TestNewLogger only asserts which levels are visible after an invalid level is
// supplied, never that the unknown_log_level WARN itself is emitted (or, for the
// empty-string case, that it is correctly suppressed).
func TestNewLogger_UnknownLevelWarning(t *testing.T) {
	tests := []struct {
		name      string
		level     string
		wantWarn  bool
		wantLevel string
	}{
		{"non-empty unrecognized level warns", "VERBOSE", true, "VERBOSE"},
		{"empty level does not warn", "", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			logger.NewLogger(tt.level, &buf)

			output := buf.String()
			gotWarn := strings.Contains(output, "event=unknown_log_level")
			if gotWarn != tt.wantWarn {
				t.Errorf("unknown_log_level WARN present=%v, want %v (output=%q)", gotWarn, tt.wantWarn, output)
			}
			if tt.wantWarn {
				wantLevelField := "level=" + tt.wantLevel
				if !strings.Contains(output, wantLevelField) {
					t.Errorf("output %q does not contain %q", output, wantLevelField)
				}
			}
		})
	}
}

func TestEffectiveLevel(t *testing.T) {
	tests := []struct {
		name  string
		level string
		want  slog.Level
	}{
		{"debug", "DEBUG", slog.LevelDebug},
		{"case insensitive debug", "debug", slog.LevelDebug},
		{"info", "INFO", slog.LevelInfo},
		{"error", "ERROR", slog.LevelError},
		{"empty resolves to INFO", "", slog.LevelInfo},
		{"unrecognized resolves to INFO", "VERBOSE", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := logger.EffectiveLevel(tt.level); got != tt.want {
				t.Errorf("EffectiveLevel(%q) = %v, want %v", tt.level, got, tt.want)
			}
		})
	}
}

func TestInitLogger(t *testing.T) {
	t.Parallel()
	l := logger.InitLogger("INFO")
	if l == nil {
		t.Fatal("InitLogger() returned nil")
	}
}

// emit writes a single record at the named level, so the tests below can cover
// every level from a table without four near-identical bodies.
func emit(l *slog.Logger, level string) {
	switch level {
	case "DEBUG":
		l.Debug("probe")
	case "INFO":
		l.Info("probe")
	case "WARN":
		l.Warn("probe")
	case "ERROR":
		l.Error("probe")
	}
}

func TestNewLogger_TimestampIsUTC(t *testing.T) {
	tests := []struct {
		name  string
		level string
	}{
		{"debug record", "DEBUG"},
		{"info record", "INFO"},
		{"warn record", "WARN"},
		{"error record", "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			emit(logger.NewLogger("DEBUG", &buf), tt.level)

			got := fieldValue(strings.TrimSpace(buf.String()), "time")
			if got == "" {
				t.Fatalf("no time field in output %q", buf.String())
			}
			if !strings.HasSuffix(got, "Z") {
				t.Errorf("time=%q does not end with Z, so it is not UTC", got)
			}
			if _, err := time.Parse(time.RFC3339, got); err != nil {
				t.Errorf("time=%q is not RFC 3339: %v", got, err)
			}
		})
	}
}

func TestWithComponent(t *testing.T) {
	tests := []struct {
		name      string
		level     string
		component string
	}{
		{"debug record", "DEBUG", logger.ComponentMain},
		{"info record", "INFO", logger.ComponentMQTT},
		{"warn record", "WARN", logger.ComponentDatabase},
		{"error record", "ERROR", logger.ComponentDatabase},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			emit(logger.WithComponent(logger.NewLogger("DEBUG", &buf), tt.component), tt.level)

			if got := fieldValue(strings.TrimSpace(buf.String()), "component"); got != tt.component {
				t.Errorf("component=%q, want %q (output=%q)", got, tt.component, buf.String())
			}
		})
	}
}
