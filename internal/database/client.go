// Package database provides a wrapper around the pgx PostgreSQL connection pool, managing connection lifecycle.
package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spydemon/mqtt2bdd/internal/config"
)

const (
	minConns        = int32(1)
	maxConns        = int32(5)
	maxConnIdleTime = 5 * time.Minute
)

// Client wraps a pgx connection pool, retaining connection metadata for logging.
type Client struct {
	pool   *pgxpool.Pool
	dsn    string // connection string including credentials; never logged
	host   string
	dbName string
	logger *slog.Logger
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
	return &Client{
		dsn:    dsn,
		host:   cfg.PostgresHost,
		dbName: cfg.PostgresDB,
		logger: l,
	}
}

// Connect establishes the pgx connection pool configured with MinConns=1 and
// MaxConns=5, then verifies connectivity with a Ping. It logs the attempt and
// outcome. Returns an error if the pool cannot be created or the initial ping
// fails, having already logged the failure with host and database context.
func (c *Client) Connect(ctx context.Context) error {
	c.logger.Info("connecting to PostgreSQL", "host", c.host, "database", c.dbName)

	poolConfig, err := pgxpool.ParseConfig(c.dsn)
	if err != nil {
		c.logger.Error("Failed to parse PostgreSQL DSN", "host", c.host, "database", c.dbName, "error", err)
		return fmt.Errorf("failed to parse PostgreSQL DSN for %s/%s: %w", c.host, c.dbName, err)
	}

	poolConfig.MinConns = minConns
	poolConfig.MaxConns = maxConns
	poolConfig.MaxConnIdleTime = maxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		c.logger.Error("PostgreSQL connection failed", "host", c.host, "database", c.dbName, "error", err)
		return fmt.Errorf("PostgreSQL connection failed for %s/%s: %w", c.host, c.dbName, err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		c.logger.Error("PostgreSQL ping failed", "host", c.host, "database", c.dbName, "error", err)
		return fmt.Errorf("PostgreSQL ping failed at %s/%s: %w", c.host, c.dbName, err)
	}

	c.pool = pool
	c.logger.Info("PostgreSQL connected", "host", c.host, "database", c.dbName)
	return nil
}

// Close shuts down the connection pool and logs disconnection.
func (c *Client) Close() {
	if c.pool != nil {
		c.pool.Close()
	}
	c.logger.Info("PostgreSQL disconnected")
}
