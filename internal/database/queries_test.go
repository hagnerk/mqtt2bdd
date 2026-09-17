package database

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestIsPermanent pins the write error classification with synthetic PostgreSQL errors, so
// no database is needed. This file is white-box (package database) because isPermanent is
// unexported. The costly mistake is a transient error classified as permanent, which
// discards a message the next attempt would have stored, hence the transient rows.
func TestIsPermanent(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "22P05 unsupported unicode escape", err: &pgconn.PgError{Code: "22P05"}, want: true},
		{name: "22P02 invalid text representation", err: &pgconn.PgError{Code: "22P02"}, want: true},
		{name: "22021 invalid byte sequence", err: &pgconn.PgError{Code: "22021"}, want: true},
		{name: "22003 numeric value out of range", err: &pgconn.PgError{Code: "22003"}, want: true},
		{name: "23502 not null violation", err: &pgconn.PgError{Code: "23502"}, want: true},
		{name: "54001 statement too complex", err: &pgconn.PgError{Code: "54001"}, want: true},
		{name: "08006 connection failure", err: &pgconn.PgError{Code: "08006"}, want: false},
		{name: "40P01 deadlock detected", err: &pgconn.PgError{Code: "40P01"}, want: false},
		{name: "42501 insufficient privilege", err: &pgconn.PgError{Code: "42501"}, want: false},
		{name: "42P01 undefined table", err: &pgconn.PgError{Code: "42P01"}, want: false},
		{name: "53100 disk full", err: &pgconn.PgError{Code: "53100"}, want: false},
		{name: "57P01 admin shutdown", err: &pgconn.PgError{Code: "57P01"}, want: false},
		{name: "XX000 internal error", err: &pgconn.PgError{Code: "XX000"}, want: false},
		{name: "wrapped permanent error", err: fmt.Errorf("insert: %w", &pgconn.PgError{Code: "22P05"}), want: true},
		{name: "plain error", err: errors.New("connection refused"), want: false},
		{name: "context deadline exceeded", err: context.DeadlineExceeded, want: false},
		{name: "nil error", err: nil, want: false},
		{name: "PgError without a code", err: &pgconn.PgError{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPermanent(tt.err); got != tt.want {
				t.Errorf("isPermanent(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
