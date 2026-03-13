# Test Strategy and Standards

## Philosophy

MQTT2BDD adopts a pragmatic testing approach aligned with its educational objectives and operational constraints. The goal is **confidence in correctness**, not coverage metrics for their own sake.

**Core principles:**
- **Test behaviour, not implementation** — tests verify what a function does, not how it does it internally
- **Prefer simple, explicit tests** — a readable failing test is more valuable than a clever passing one
- **Avoid over-mocking** — mock at system boundaries (MQTT broker, PostgreSQL), not between internal packages
- **Integration tests are first-class** — the critical path (MQTT → channel → PostgreSQL) must be validated end-to-end

## Test Types

### Unit Tests

**Scope:** Individual functions and methods in isolation, without external dependencies (no broker, no database).

**When to use:** Logic that can be verified purely in memory — configuration parsing, error wrapping, message construction, JSON validation.

**Tools:** Go stdlib `testing` package only. No third-party assertion libraries (testify, gomega, etc.) — standard `if got != want` comparisons keep tests dependency-free and idiomatic.

**Style: Table-Driven Tests**

All unit tests use the table-driven pattern — the idiomatic Go approach for exhaustive case coverage:

```go
func TestLoadConfig(t *testing.T) {
    tests := []struct {
        name    string
        env     map[string]string
        want    *Config
        wantErr bool
    }{
        {
            name: "all required vars set",
            env: map[string]string{
                "MQTT_BROKER":       "localhost",
                "MQTT_PORT":         "1883",
                "POSTGRES_HOST":     "localhost",
                "POSTGRES_PORT":     "5432",
                "POSTGRES_DB":       "mqtt2bdd",
                "POSTGRES_USER":     "mqtt2bdd",
                "POSTGRES_PASSWORD": "secret",
            },
            want: &Config{
                MQTTBroker:       "localhost",
                MQTTPort:         1883,
                PostgresHost:     "localhost",
                PostgresPort:     5432,
                PostgresDB:       "mqtt2bdd",
                PostgresUser:     "mqtt2bdd",
                PostgresPassword: "secret",
                LogLevel:         "INFO", // default
                BufferSize:       1000,   // default
            },
            wantErr: false,
        },
        {
            name:    "missing POSTGRES_PASSWORD",
            env:     map[string]string{"MQTT_BROKER": "localhost" /*, ... */},
            want:    nil,
            wantErr: true,
        },
        {
            name:    "invalid MQTT_PORT (not a number)",
            env:     map[string]string{"MQTT_PORT": "not-a-port" /*, ... */},
            want:    nil,
            wantErr: true,
        },
        {
            name: "custom buffer size",
            env:  map[string]string{"BUFFER_SIZE": "5000" /*, ... */},
            want: &Config{BufferSize: 5000 /*, ... */},
            wantErr: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            for k, v := range tt.env {
                t.Setenv(k, v) // t.Setenv auto-restores after each subtest
            }

            got, err := LoadConfig()

            if (err != nil) != tt.wantErr {
                t.Errorf("LoadConfig() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !tt.wantErr && *got != *tt.want {
                t.Errorf("LoadConfig() = %+v, want %+v", got, tt.want)
            }
        })
    }
}
```

**Key conventions:**
- Use `t.Setenv()` (Go 1.17+) to set environment variables — it automatically cleans up after each subtest
- Use `t.Run(tt.name, ...)` so each case runs independently and can be targeted with `-run`
- Test file name: `xxx_test.go` in the same directory as the code under test

### Integration Tests

**Scope:** Interactions between MQTT2BDD and real external services (PostgreSQL, Mosquitto). Validates the full message pipeline from MQTT publish to database persistence.

**Infrastructure:** `test/docker-compose.yml` — isolated containers with no ports exposed to the host (see Infrastructure section).

**Build tag:** Integration tests are gated behind the `integration` build tag to keep `go test ./...` fast in CI:

```go
//go:build integration

package database_test

import "testing"

func TestInsertMessage_Integration(t *testing.T) {
    // Requires real PostgreSQL — only runs with -tags=integration
    ...
}
```

**Integration test scenarios (minimum required):**

| Test | Description |
|------|-------------|
| `TestFullPipeline` | Publish MQTT message → verify row inserted in `sensor_metrics` |
| `TestDatabaseReconnect` | Simulate DB outage → verify messages buffered → verify recovery |
| `TestDuplicateMessage` | Insert same (sensor, date) twice → verify `ON CONFLICT DO NOTHING` behaviour |
| `TestGracefulShutdown` | Send SIGTERM → verify buffered messages flushed before exit |

## Coverage

Coverage is a **signal**, not a target. A package with well-chosen cases that exercise all meaningful behaviours is preferable to one padded with trivial tests to hit an arbitrary percentage.

Use coverage to **identify untested paths**, not to measure quality:

```bash
# Per-package coverage report
go test -cover ./...

# HTML coverage report — highlights untested lines visually
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

If a line is uncovered, ask: "Is this a missing test, or dead code?" — both are worth investigating.

## Test Organization

**File naming:**
```
internal/config/
    config.go
    config_test.go          # unit tests

internal/database/
    client.go
    queries.go
    client_test.go          # unit tests
    integration_test.go     # //go:build integration
```

**Package convention:**

Use **white-box tests** (same package) for testing unexported behaviour:
```go
package config // same package as config.go
```

Use **black-box tests** (external `_test` package) for testing the public API in isolation:
```go
package config_test // external test package
```

Prefer black-box tests for exported functions — they verify the API as a consumer would use it, and prevent tests from coupling to unexported implementation details.

## Test Helpers

Avoid third-party helper libraries. Use simple local helpers when setup is repeated:

```go
// testhelpers_test.go (within the test package)

// mustConfig returns a valid Config for tests, applying any env overrides.
// Calls t.Fatal on error — acceptable in test setup, not in production code.
func mustConfig(t *testing.T, overrides map[string]string) *Config {
    t.Helper()
    defaults := map[string]string{
        "MQTT_BROKER":       "localhost",
        "MQTT_PORT":         "1883",
        "POSTGRES_HOST":     "localhost",
        "POSTGRES_PORT":     "5432",
        "POSTGRES_DB":       "mqtt2bdd",
        "POSTGRES_USER":     "mqtt2bdd",
        "POSTGRES_PASSWORD": "test_password",
    }
    for k, v := range overrides {
        defaults[k] = v
    }
    for k, v := range defaults {
        t.Setenv(k, v)
    }
    cfg, err := LoadConfig()
    if err != nil {
        t.Fatalf("mustConfig: %v", err)
    }
    return cfg
}
```

`t.Helper()` ensures that failure output points to the calling test line, not inside the helper.

## What Not to Test

- **Third-party library behaviour** — do not test that Paho auto-reconnects; test that your reconnection callback is invoked correctly
- **Trivial getters/setters** — if a method simply returns a field value, it does not need a dedicated test
- **Log output format** — log messages are operational, not contractual; they will change over time

## Running Tests (Quick Reference)

```bash
# Unit tests only (default — fast, no external deps)
go test ./...

# Unit tests with verbose output
go test -v ./...

# Specific package
go test ./internal/config/...

# Specific test by name
go test -run TestLoadConfig ./internal/config/...

# With coverage
go test -cover ./...

# Integration tests (requires test environment up)
./test/run-integration-tests.sh
```

---
