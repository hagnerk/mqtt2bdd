package main

import (
	"testing"
	"time"
)

// TestBufferUtilizationPercent uses Go's table-driven test idiom: one slice of
// cases (a name plus inputs plus the expected output), run through a single
// t.Run per case. Each subtest reports pass/fail independently, and any one of
// them can be re-run in isolation with `go test -run TestBufferUtilizationPercent/full_buffer`
// without touching the others — the pattern used throughout this codebase's
// test suite (config_test.go, logger_test.go), called out explicitly here once.
func TestBufferUtilizationPercent(t *testing.T) {
	tests := []struct {
		name     string
		used     int
		capacity int
		want     float64
	}{
		{"unbuffered channel yields zero, not NaN", 0, 0, 0},
		{"empty buffer", 0, 1000, 0},
		{"exactly at the high-water mark", 800, 1000, 80},
		{"just above the high-water mark", 801, 1000, 80.1},
		{"full buffer", 1000, 1000, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Exact comparison is deliberate: every expected value is representable
			// after the one-decimal rounding the function applies.
			if got := bufferUtilizationPercent(tt.used, tt.capacity); got != tt.want {
				t.Errorf("bufferUtilizationPercent(%d, %d) = %v, want %v", tt.used, tt.capacity, got, tt.want)
			}
		})
	}
}

func TestConnectionStatus(t *testing.T) {
	tests := []struct {
		name      string
		connected bool
		want      string
	}{
		{"connected", true, "connected"},
		{"disconnected", false, "disconnected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := connectionStatus(tt.connected); got != tt.want {
				t.Errorf("connectionStatus(%v) = %q, want %q", tt.connected, got, tt.want)
			}
		})
	}
}

func TestWriteAgeSeconds(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		lastWrite time.Time
		want      int64
	}{
		{"never written yet (zero time)", time.Time{}, -1},
		{"written 48 seconds ago", now.Add(-48 * time.Second), 48},
		{"written this instant", now, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel() // pure function, no shared state
			if got := writeAgeSeconds(tt.lastWrite, now); got != tt.want {
				t.Errorf("writeAgeSeconds(%v, %v) = %d, want %d", tt.lastWrite, now, got, tt.want)
			}
		})
	}
}
