//go:build integration

package database_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spydemon/mqtt2bdd/internal/config"
	"github.com/spydemon/mqtt2bdd/internal/database"
	"github.com/spydemon/mqtt2bdd/internal/logger"
)

func TestMain(m *testing.M) {
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: load config: %v\n", err)
		os.Exit(1)
	}
	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s",
		cfg.PostgresHost, cfg.PostgresPort, cfg.PostgresDB, cfg.PostgresUser, cfg.PostgresPassword)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: open pool: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	if _, err := pool.Exec(context.Background(), "TRUNCATE TABLE sensor_metrics"); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: truncate before tests: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	if _, err := pool.Exec(context.Background(), "TRUNCATE TABLE sensor_metrics"); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: truncate after tests: %v\n", err)
		os.Exit(1)
	}

	os.Exit(code)
}

func newTestClient(t *testing.T) (*database.Client, *config.Config) {
	t.Helper()
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	l := logger.InitLogger("DEBUG")
	client := database.NewClient(cfg, l)
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client, cfg
}

func newVerificationPool(t *testing.T, cfg *config.Config) *pgxpool.Pool {
	t.Helper()
	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s",
		cfg.PostgresHost, cfg.PostgresPort, cfg.PostgresDB, cfg.PostgresUser, cfg.PostgresPassword)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open verification pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestInsertMessage_Success(t *testing.T) {
	client, cfg := newTestClient(t)
	pool := newVerificationPool(t, cfg)

	sensor := "test/integration/success"
	ts := time.Now().UTC().Truncate(time.Microsecond)
	metrics := json.RawMessage(`{"temperature": 21.3}`)

	if err := client.InsertMessage(context.Background(), sensor, ts, metrics); err != nil {
		t.Fatalf("InsertMessage returned unexpected error: %v", err)
	}

	var count int
	err := pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM sensor_metrics WHERE sensor = $1 AND date = $2",
		sensor, ts).Scan(&count)
	if err != nil {
		t.Fatalf("verification query failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row in sensor_metrics, got %d", count)
	}
}

func TestInsertMessage_DuplicateConflict(t *testing.T) {
	client, cfg := newTestClient(t)
	pool := newVerificationPool(t, cfg)

	sensor := "test/integration/duplicate"
	ts := time.Now().UTC().Truncate(time.Microsecond)
	metrics := json.RawMessage(`{"temperature": 22.0}`)

	if err := client.InsertMessage(context.Background(), sensor, ts, metrics); err != nil {
		t.Fatalf("first insert failed: %v", err)
	}
	if err := client.InsertMessage(context.Background(), sensor, ts, metrics); err != nil {
		t.Fatalf("second insert (duplicate) returned unexpected error: %v", err)
	}

	var count int
	err := pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM sensor_metrics WHERE sensor = $1 AND date = $2",
		sensor, ts).Scan(&count)
	if err != nil {
		t.Fatalf("verification query failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 row after duplicate insert, got %d", count)
	}
}

func TestInsertMessage_SensorTruncation(t *testing.T) {
	client, cfg := newTestClient(t)
	pool := newVerificationPool(t, cfg)

	sensor := strings.Repeat("a", 300)
	expectedSensor := strings.Repeat("a", 255)
	ts := time.Now().UTC().Truncate(time.Microsecond)
	metrics := json.RawMessage(`{"value": 1}`)

	if err := client.InsertMessage(context.Background(), sensor, ts, metrics); err != nil {
		t.Fatalf("InsertMessage with long sensor returned unexpected error: %v", err)
	}

	var count int
	err := pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM sensor_metrics WHERE sensor = $1 AND date = $2",
		expectedSensor, ts).Scan(&count)
	if err != nil {
		t.Fatalf("verification query failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row with truncated sensor, got %d", count)
	}
}

func TestInsertMessage_Error(t *testing.T) {
	client, cfg := newTestClient(t)
	pool := newVerificationPool(t, cfg)

	sensor := "test/integration/error"
	ts := time.Now().UTC().Truncate(time.Microsecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := client.InsertMessage(ctx, sensor, ts, json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error with cancelled context, got nil")
	}

	var count int
	err := pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM sensor_metrics WHERE sensor = $1 AND date = $2",
		sensor, ts).Scan(&count)
	if err != nil {
		t.Fatalf("verification query failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no row written after failed insert, got %d", count)
	}
}
