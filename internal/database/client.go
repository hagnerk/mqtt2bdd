// Package database provides a wrapper around the pgx PostgreSQL connection pool, managing connection lifecycle.
package database

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spydemon/mqtt2bdd/internal/config"
	"github.com/spydemon/mqtt2bdd/internal/logger"
)

const (
	// minConns keeps at least one connection open at all times, so the first
	// insert after an idle period does not pay the cost of establishing a new
	// connection before it can run.
	minConns = int32(1)
	// maxConns is sized for a single writer goroutine (InsertMessage is only ever
	// called from dbWriterLoop), not for concurrent load; five gives headroom for
	// the occasional overlapping health-check query without over-provisioning
	// connections the workload never uses.
	maxConns = int32(5)
	// maxConnIdleTime recycles idle connections after five minutes so a
	// long-lived pool does not hold connections the database side may have
	// silently dropped (e.g. after a network blip or a Postgres-side timeout).
	maxConnIdleTime = 5 * time.Minute
)

// Client wraps a pgx connection pool, retaining connection metadata for logging.
//
// The health fields below are written by the writer goroutine (through InsertMessage)
// and read by the health-check goroutine, so they are atomics rather than plain fields.
// A Client is always handled as *Client, so they are never copied.
type Client struct {
	pool   *pgxpool.Pool
	dsn    string // connection string including credentials; never logged
	host   string
	dbName string
	logger *slog.Logger

	connected   atomic.Bool   // outcome of the most recent database interaction
	lastWriteAt atomic.Int64  // Unix nanoseconds of the last successful write; 0 means never
	writeCount  atomic.Uint64 // monotonic count of successful writes since startup
}

// NewClient constructs a Client with the connection string derived from cfg.
// It does not establish a connection; call Connect to open the pool.
func NewClient(cfg *config.Config, l *slog.Logger) *Client {
	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s",
		cfg.PostgresHost,
		cfg.PostgresPort,
		cfg.PostgresDB,
		cfg.PostgresUser,
		cfg.PostgresPassword,
	)
	// The component, host and database context is bound once here, so no call site repeats it.
	return &Client{
		dsn:    dsn,
		host:   cfg.PostgresHost,
		dbName: cfg.PostgresDB,
		logger: l.With("component", logger.ComponentDatabase, "host", cfg.PostgresHost, "database", cfg.PostgresDB),
	}
}

// Connect establishes the pgx connection pool configured with MinConns=1 and
// MaxConns=5, then verifies connectivity with a Ping. It logs the attempt and
// outcome. Returns an error if the pool cannot be created or the initial ping
// fails, having already logged the failure with host and database context.
func (c *Client) Connect(ctx context.Context) error {
	c.logger.Info("connecting to PostgreSQL", "event", "connecting")

	poolConfig, err := pgxpool.ParseConfig(c.dsn)
	if err != nil {
		c.logger.Error("Failed to parse PostgreSQL DSN", "event", "connect_failed", "operation", "parse_dsn", "error", err)
		return fmt.Errorf("failed to parse PostgreSQL DSN for %s/%s: %w", c.host, c.dbName, err)
	}

	poolConfig.MinConns = minConns
	poolConfig.MaxConns = maxConns
	poolConfig.MaxConnIdleTime = maxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		c.logger.Error("PostgreSQL connection failed", "event", "connect_failed", "operation", "connect", "error", err)
		return fmt.Errorf("PostgreSQL connection failed for %s/%s: %w", c.host, c.dbName, err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		c.logger.Error("PostgreSQL ping failed", "event", "connect_failed", "operation", "ping", "error", err)
		return fmt.Errorf("PostgreSQL ping failed at %s/%s: %w", c.host, c.dbName, err)
	}

	c.pool = pool
	// The startup ping is what seeds the health status: it proves reachability before
	// any message has been written, so IsConnected() is meaningful from the first tick.
	c.connected.Store(true)
	c.logger.Info("PostgreSQL connected", "event", "connected")
	c.LogPoolStats()
	return nil
}

// Close shuts down the connection pool and logs disconnection.
func (c *Client) Close() {
	c.LogPoolStats()
	if c.pool != nil {
		c.pool.Close()
	}
	// A torn-down pool is not connected, whatever the last write returned. This keeps
	// IsConnected()'s contract true unconditionally rather than only for as long as the
	// shutdown ordering happens to keep readers away.
	c.connected.Store(false)
	c.logger.Info("PostgreSQL disconnected", "event", "disconnected")
}

// IsConnected reports whether the most recent database interaction succeeded: the
// startup ping, or the last write attempted since. It performs no network call and
// costs a single atomic load, so it is safe to call at any cadence.
func (c *Client) IsConnected() bool {
	return c.connected.Load()
}

// LastWriteAt returns the time of the last successful write, or the zero time.Time
// when no write has succeeded since startup.
func (c *Client) LastWriteAt() time.Time {
	ns := c.lastWriteAt.Load()
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

// WriteCount returns the total number of successful writes since startup. It is
// monotonic and never reset; a per-interval rate is a delta the caller computes.
func (c *Client) WriteCount() uint64 {
	return c.writeCount.Load()
}

// LogPoolStats logs the current connection pool statistics at DEBUG level.
// It is a no-op when the pool has not been opened.
func (c *Client) LogPoolStats() {
	if c.pool == nil {
		return
	}
	stat := c.pool.Stat()
	c.logger.Debug("PostgreSQL pool stats",
		"event", "pool_stats",
		"acquired_conns", stat.AcquiredConns(),
		"idle_conns", stat.IdleConns(),
		"total_conns", stat.TotalConns(),
		"max_conns", stat.MaxConns(),
	)
}
