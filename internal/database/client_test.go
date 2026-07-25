package database_test

import (
	"io"
	"testing"

	"github.com/spydemon/mqtt2bdd/internal/config"
	"github.com/spydemon/mqtt2bdd/internal/database"
	"github.com/spydemon/mqtt2bdd/internal/logger"
)

// TestClientHealthZeroValues pins the health contract of a client that has never
// connected: the accessors must report "nothing has happened yet" rather than a
// plausible-looking default, since the health check distinguishes the two.
// NewClient opens no connection, so this needs no database.
func TestClientHealthZeroValues(t *testing.T) {
	cfg := &config.Config{PostgresHost: "localhost", PostgresPort: 5432, PostgresDB: "test", PostgresUser: "test"}
	c := database.NewClient(cfg, logger.NewLogger("INFO", io.Discard))

	t.Run("not connected before Connect", func(t *testing.T) {
		if got := c.IsConnected(); got {
			t.Errorf("IsConnected() = %v, want false", got)
		}
	})

	t.Run("last write time is zero before any write", func(t *testing.T) {
		if got := c.LastWriteAt(); !got.IsZero() {
			t.Errorf("LastWriteAt() = %v, want the zero time", got)
		}
	})

	t.Run("write count is zero before any write", func(t *testing.T) {
		if got := c.WriteCount(); got != 0 {
			t.Errorf("WriteCount() = %d, want 0", got)
		}
	})
}
