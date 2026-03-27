package logger_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spydemon/mqtt2bdd/internal/logger"
)

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

func TestInitLogger(t *testing.T) {
	l := logger.InitLogger("INFO")
	if l == nil {
		t.Fatal("InitLogger() returned nil")
	}
}
