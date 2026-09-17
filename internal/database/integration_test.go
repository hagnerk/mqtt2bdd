//go:build integration

package database_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hagnerk/mqtt2bdd/internal/config"
	"github.com/hagnerk/mqtt2bdd/internal/database"
	"github.com/hagnerk/mqtt2bdd/internal/logger"
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

	if _, err := pool.Exec(context.Background(), "TRUNCATE TABLE sensor_metrics"); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: truncate before tests: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	pool.Exec(context.Background(), "TRUNCATE TABLE sensor_metrics") // best-effort cleanup

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

	err := client.InsertMessage(ctx, sensor, ts, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error with cancelled context, got nil")
	}
	// A transient error classified as permanent would discard a message the next
	// attempt could have stored.
	if errors.Is(err, database.ErrRejected) {
		t.Fatalf("a cancelled context is transient, got an error wrapping ErrRejected: %v", err)
	}

	var count int
	err = pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM sensor_metrics WHERE sensor = $1 AND date = $2",
		sensor, ts).Scan(&count)
	if err != nil {
		t.Fatalf("verification query failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no row written after failed insert, got %d", count)
	}
}

// TestInsertMessage_Rejected covers a payload that passes json.Valid but that PostgreSQL
// refuses for its content: a number beyond the numeric range (SQLSTATE 22003). It is the
// only direct evidence that a server rejection leaves the client connected and the write
// fields untouched, which the subprocess test cannot observe before its first health tick.
func TestInsertMessage_Rejected(t *testing.T) {
	client, cfg := newTestClient(t)
	pool := newVerificationPool(t, cfg)

	sensor := "test/integration/rejected"
	ts := time.Now().UTC().Truncate(time.Microsecond)
	writeCountBefore := client.WriteCount()
	lastWriteBefore := client.LastWriteAt()
	rejectedBefore := client.RejectedCount()

	err := client.InsertMessage(context.Background(), sensor, ts, json.RawMessage(`{"v":1e1000000}`))

	if !errors.Is(err, database.ErrRejected) {
		t.Fatalf("expected an error wrapping ErrRejected, got %v", err)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected the error to still wrap a *pgconn.PgError, got %v", err)
	}
	if pgErr.Code != "22003" {
		t.Errorf("SQLSTATE = %s, want 22003", pgErr.Code)
	}
	if !client.IsConnected() {
		t.Error("IsConnected() = false after a server rejection, want true: the database answered")
	}
	if got := client.WriteCount(); got != writeCountBefore {
		t.Errorf("WriteCount() = %d, want %d: nothing was written", got, writeCountBefore)
	}
	if got := client.LastWriteAt(); !got.Equal(lastWriteBefore) {
		t.Errorf("LastWriteAt() = %v, want %v: nothing was written", got, lastWriteBefore)
	}
	if got := client.RejectedCount(); got != rejectedBefore+1 {
		t.Errorf("RejectedCount() = %d, want %d", got, rejectedBefore+1)
	}

	var count int
	err = pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM sensor_metrics WHERE sensor = $1", sensor).Scan(&count)
	if err != nil {
		t.Fatalf("verification query failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no row written after a rejection, got %d", count)
	}
}

// TestInsertMessage_Sanitized covers a payload PostgreSQL would refuse three times over
// (SQLSTATE 22P05, 22P02 and 22021) and that is stored once repaired: each defect reads
// back as U+FFFD, while the literal \\u0000 text and the valid surrogate pair beside them
// are stored as published.
func TestInsertMessage_Sanitized(t *testing.T) {
	client, cfg := newTestClient(t)
	pool := newVerificationPool(t, cfg)

	sensor := "test/integration/sanitized"
	ts := time.Now().UTC().Truncate(time.Microsecond)
	writeCountBefore := client.WriteCount()
	rejectedBefore := client.RejectedCount()

	// JSON escapes in raw strings, the invalid byte in an interpreted one.
	payload := `{"nul":"x\u0000y","lone":"x\ud83dy","raw":"x` + "\xff" + `y","literal":"\\u0000","pair":"\ud83d\ude00"}`

	err := client.InsertMessage(context.Background(), sensor, ts, json.RawMessage(payload))

	if err != nil {
		t.Fatalf("InsertMessage() error = %v, want nil: the payload should have been repaired", err)
	}
	if !client.IsConnected() {
		t.Error("IsConnected() = false after a successful write, want true")
	}
	if got := client.WriteCount(); got != writeCountBefore+1 {
		t.Errorf("WriteCount() = %d, want %d", got, writeCountBefore+1)
	}
	if got := client.RejectedCount(); got != rejectedBefore {
		t.Errorf("RejectedCount() = %d, want %d: a repair is not a rejection", got, rejectedBefore)
	}

	tests := []struct {
		key  string
		want string
	}{
		{"nul", "x\uFFFDy"},
		{"lone", "x\uFFFDy"},
		{"raw", "x\uFFFDy"},
		{"literal", `\u0000`},
		{"pair", "\U0001F600"},
	}
	for _, tt := range tests {
		var got string
		err := pool.QueryRow(context.Background(),
			"SELECT metrics->>$2 FROM sensor_metrics WHERE sensor = $1", sensor, tt.key).Scan(&got)
		if err != nil {
			t.Fatalf("verification query for key %q failed: %v", tt.key, err)
		}
		if got != tt.want {
			t.Errorf("metrics->>%q = %q, want %q", tt.key, got, tt.want)
		}
	}
}
