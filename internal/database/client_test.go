package database_test

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/spydemon/mqtt2bdd/internal/config"
	"github.com/spydemon/mqtt2bdd/internal/database"
	"github.com/spydemon/mqtt2bdd/internal/logger"
)

// *pgxpool.Pool is a concrete struct, not an interface, so it cannot be faked
// the same way internal/mqtt fakes mqtt.Client — there is no seam to substitute
// a test double behind. That is why this file's new tests below target
// Connect's two failure paths reachable BEFORE the pool is ever used for a
// query (DSN parsing, a guaranteed-unreachable listener), and leave
// InsertMessage's actual query behaviour (success, duplicate, truncation,
// error) to integration_test.go's existing suite, which runs against a real
// PostgreSQL — the system-boundary line test-strategy-and-standards.md's
// Philosophy section draws ("mock at system boundaries, not between internal
// packages").

// TestClientHealthZeroValues pins the health contract of a client that has never
// connected: the accessors must report "nothing has happened yet" rather than a
// plausible-looking default, since the health check distinguishes the two.
// NewClient opens no connection, so this needs no database.
func TestClientHealthZeroValues(t *testing.T) {
	cfg := &config.Config{PostgresHost: "localhost", PostgresPort: 5432, PostgresDB: "test", PostgresUser: "test"}
	c := database.NewClient(cfg, logger.NewLogger("INFO", io.Discard))

	t.Run("not connected before Connect", func(t *testing.T) {
		t.Parallel()
		if got := c.IsConnected(); got {
			t.Errorf("IsConnected() = %v, want false", got)
		}
	})

	t.Run("last write time is zero before any write", func(t *testing.T) {
		t.Parallel()
		if got := c.LastWriteAt(); !got.IsZero() {
			t.Errorf("LastWriteAt() = %v, want the zero time", got)
		}
	})

	t.Run("write count is zero before any write", func(t *testing.T) {
		t.Parallel()
		if got := c.WriteCount(); got != 0 {
			t.Errorf("WriteCount() = %d, want 0", got)
		}
	})
}

// TestConnect_InvalidDSN exercises Connect's pgxpool.ParseConfig failure branch
// (client.go:66-70): a PostgresHost containing a raw, unquoted space breaks the
// "host=%s port=%d ..." keyword/value DSN NewClient builds, which pgx rejects
// with no network call at all — no live database needed.
func TestConnect_InvalidDSN(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{PostgresHost: "invalid host", PostgresPort: 5432, PostgresDB: "test", PostgresUser: "test"}
	c := database.NewClient(cfg, logger.NewLogger("ERROR", io.Discard))

	err := c.Connect(context.Background())
	if err == nil {
		t.Fatal("Connect() = nil, want an error for an invalid DSN")
	}
	if c.IsConnected() {
		t.Error("IsConnected() = true after a failed Connect(), want false")
	}
}

// TestConnect_UnreachableHost exercises Connect's pool.Ping failure branch
// (client.go:82-86). A net.Listener is opened on an OS-assigned free port, then
// closed immediately, guaranteeing nothing listens on that exact port for the
// rest of the test — deterministic connection-refused, no live database, no
// coordination with any Docker stack.
func TestConnect_UnreachableHost(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to open a listener to reserve a free port: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	if err := ln.Close(); err != nil {
		t.Fatalf("failed to close the reserving listener: %v", err)
	}

	cfg := &config.Config{PostgresHost: addr.IP.String(), PostgresPort: addr.Port, PostgresDB: "test", PostgresUser: "test"}
	c := database.NewClient(cfg, logger.NewLogger("ERROR", io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Connect(ctx); err == nil {
		t.Fatal("Connect() = nil, want an error for an unreachable host")
	}
	if c.IsConnected() {
		t.Error("IsConnected() = true after a failed Connect(), want false")
	}
}

// TestClose_BeforeConnect asserts the c.pool == nil guard at client.go:100: Close
// on a NewClient-constructed, never-Connect-ed Client must not panic.
func TestClose_BeforeConnect(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{PostgresHost: "localhost", PostgresPort: 5432, PostgresDB: "test", PostgresUser: "test"}
	c := database.NewClient(cfg, logger.NewLogger("ERROR", io.Discard))

	c.Close() // must not panic
}

// TestLogPoolStats_NoopBeforeConnect asserts the c.pool == nil guard at
// client.go:136. Per test-strategy-and-standards.md#what-not-to-test, the log
// line's content is not asserted — only that the no-op guard holds.
func TestLogPoolStats_NoopBeforeConnect(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{PostgresHost: "localhost", PostgresPort: 5432, PostgresDB: "test", PostgresUser: "test"}
	c := database.NewClient(cfg, logger.NewLogger("ERROR", io.Discard))

	c.LogPoolStats() // must not panic
}
