# Error Handling Strategy

Defining comprehensive error handling approach for MQTT2BDD following Go best practices and operational requirements.

## General Error Handling Philosophy

**Core Principles:**

1. **Explicit Error Returns (No Exceptions/Panics):**
   - All functions return `error` as last return value
   - Errors are values, not exceptions
   - No `panic()` in normal operation (only for programmer errors during development)

2. **Fail-Fast at Startup:**
   - Configuration errors → log FATAL and exit immediately
   - Connection failures (MQTT/DB) at startup → log FATAL and exit
   - Invalid setup detected early, before message processing begins

3. **Resilient at Runtime:**
   - Transient failures (network, DB outage) → automatic retry with backoff
   - Application continues running during failures
   - Errors logged with context, never silently swallowed

4. **Graceful Degradation:**
   - MQTT disconnected → buffer messages, retry connection
   - Database unavailable → buffer in channel (up to 1000 messages)
   - Overflow scenarios → log WARNING, apply backpressure

5. **Comprehensive Logging:**
   - All errors logged with structured context (component, operation, error details)
   - Error severity levels: DEBUG (retries), INFO (recoveries), ERROR (failures), FATAL (unrecoverable)

**Error Model (Go Standard Pattern):**

```go
// Standard Go error handling pattern
func DoSomething(ctx context.Context) error {
    result, err := operation()
    if err != nil {
        return fmt.Errorf("failed to do something: %w", err)  // Wrap with context
    }
    return nil
}

// Usage
if err := DoSomething(ctx); err != nil {
    logger.Error("Operation failed", "error", err, "component", "main")
    // Decide: retry, skip, or fail
}
```

**No Custom Exception Hierarchy:**
- Use standard `error` interface
- Wrap errors with `fmt.Errorf(..., %w, err)` for context
- Use `errors.Is()` and `errors.As()` for error type checking
- Sentinel errors for specific conditions: `var ErrRetryable = errors.New("retryable error")`

## Error Categories and Handling

### 1. Startup Errors (Fail-Fast)

**Category:** Configuration, initialization, connection establishment

**Examples:**
- Missing environment variable: `POSTGRES_PASSWORD not set`
- Invalid configuration: `MQTT_PORT must be 1-65535`
- Database connection failure: `connection refused postgres:5432`
- MQTT broker unreachable: `connection timeout mosquitto:1883`

**Handling Strategy:**

```go
func main() {
    // Load configuration
    cfg, err := config.LoadConfig()
    if err != nil {
        logger.Error("Configuration error", "error", err)
        os.Exit(1)  // Fail-fast
    }

    // Connect to MQTT
    mqttClient := mqtt.NewClient(cfg, logger)
    if err := mqttClient.Connect(ctx); err != nil {
        logger.Error("MQTT connection failed at startup", "error", err, "broker", cfg.MQTTBroker)
        os.Exit(1)  // Fail-fast
    }

    // Connect to database
    dbClient := database.NewClient(cfg, logger)
    if err := dbClient.Connect(ctx); err != nil {
        logger.Error("Database connection failed at startup", "error", err, "host", cfg.PostgresHost)
        os.Exit(1)  // Fail-fast
    }

    logger.Info("Startup complete", "version", Version)
}
```

**Rationale:**
- Better to fail immediately than run in degraded state
- Clear error messages guide operator to fix configuration
- Container orchestration (Docker restart policy) handles restarts
- Prevents partial initialization (e.g., MQTT connected but DB not)

**Example Error Output (TextHandler format):**
```
2026-02-13T14:30:00Z ERROR Configuration error error="POSTGRES_PASSWORD not set (set via environment variable)"
2026-02-13T14:30:00Z ERROR MQTT connection failed at startup error="dial tcp mosquitto:1883: connection refused" broker=mosquitto:1883
2026-02-13T14:30:00Z ERROR Database connection failed at startup error="connection refused" host=postgres.local
```

### 2. Runtime Errors - Transient (Auto-Retry)

**Category:** Network failures, temporary service unavailability

**Examples:**
- MQTT broker disconnected (network blip)
- Database connection lost (PostgreSQL restart)
- INSERT fails due to connection error

**Handling Strategy: Automatic Retry with Fixed Interval**

**MQTT Reconnection (Paho Auto-Reconnect):**

```go
func NewClient(cfg *Config, logger *slog.Logger) *Client {
    opts := mqtt.NewClientOptions()
    opts.AddBroker(fmt.Sprintf("tcp://%s:%d", cfg.MQTTBroker, cfg.MQTTPort))

    // Auto-reconnect configuration
    opts.SetAutoReconnect(true)
    opts.SetMaxReconnectInterval(10 * time.Second)
    opts.SetConnectRetryInterval(10 * time.Second)

    // Connection lost callback
    opts.SetOnConnectionLost(func(client mqtt.Client, err error) {
        logger.Error("MQTT connection lost",
            "error", err,
            "component", "mqtt",
            "broker", cfg.MQTTBroker,
        )
    })

    // Reconnection callback
    opts.SetOnConnect(func(client mqtt.Client) {
        logger.Info("MQTT reconnected",
            "component", "mqtt",
            "broker", cfg.MQTTBroker,
        )
        // Re-subscribe to topics
        client.Subscribe("#", 0, messageHandler)
        logger.Info("Resubscribed to all topics", "component", "mqtt")
    })

    return &Client{client: mqtt.NewClient(opts), logger: logger}
}
```

**Database Reconnection (Infinite Retry in DB Writer Goroutine):**

```go
// DB Writer goroutine (in main.go)
func dbWriterLoop(msgChan <-chan Message, dbClient *database.Client, logger *slog.Logger) {
    for msg := range msgChan {
        // Retry indefinitely until success
        for {
            err := dbClient.InsertMessage(context.Background(), msg.Topic, msg.Timestamp, msg.Payload)

            if err == nil {
                // Success - move to next message
                break
            }

            // Failed - log and retry after 10 seconds
            logger.Warn("Database write failed, retrying in 10s",
                "error", err,
                "sensor", msg.Topic,
            )
            time.Sleep(10 * time.Second)
            // Loop continues - retry same message
        }
    }
    logger.Info("DB writer goroutine exiting (channel closed)")
}

// InsertMessage - simple, no internal retry logic
func (c *Client) InsertMessage(ctx context.Context, sensor string, timestamp time.Time, metrics json.RawMessage) error {
    query := `
        INSERT INTO sensor_metrics (sensor, date, metrics)
        VALUES ($1, $2, $3)
        ON CONFLICT (sensor, date) DO NOTHING
    `

    commandTag, err := c.pool.Exec(ctx, query, sensor, timestamp, metrics)
    if err != nil {
        return fmt.Errorf("database insert failed: %w", err)
    }

    // Check for duplicate
    if commandTag.RowsAffected() == 0 {
        c.logger.Warn("Duplicate message ignored",
            "sensor", sensor,
            "timestamp", timestamp.Format(time.RFC3339),
        )
    } else {
        c.logger.Debug("Message persisted",
            "sensor", sensor,
            "timestamp", timestamp.Format(time.RFC3339),
        )
    }

    return nil
}
```

**Retry Configuration:**
- **Interval:** Fixed 10 seconds (simple, predictable)
- **Max attempts:** Infinite for *transient* database write failures; permanent rejections are never retried (see §3)
- **Max attempts:** Infinite for MQTT reconnection (Paho handles internally)

**Architecture Rationale:**
- **Separation of concerns:** `InsertMessage()` does the INSERT, caller handles retry strategy
- **Resilience:** DB Writer goroutine never gives up on a message while its failure is transient (retries indefinitely)
- **Observability:** Each retry logged with context
- **Simplicity:** Fixed interval avoids exponential backoff complexity

**Example Log Output During Database Outage:**
```
2026-02-13T14:30:00Z WARN Database write failed, retrying in 10s error="connection refused" sensor=bedroom/temp
2026-02-13T14:30:10Z WARN Database write failed, retrying in 10s error="connection refused" sensor=bedroom/temp
2026-02-13T14:30:20Z WARN Database write failed, retrying in 10s error="connection refused" sensor=bedroom/temp
2026-02-13T14:30:30Z DEBUG Message persisted sensor=bedroom/temp timestamp=2026-02-13T14:30:00Z
2026-02-13T14:30:30Z DEBUG Message persisted sensor=living_room/temp timestamp=2026-02-13T14:30:05Z
```

**Example Log Output During MQTT Outage:**
```
2026-02-13T14:35:00Z ERROR MQTT connection lost error="EOF" component=mqtt broker=mosquitto:1883
2026-02-13T14:35:10Z INFO Attempting MQTT reconnection component=mqtt
2026-02-13T14:35:20Z INFO MQTT reconnected component=mqtt broker=mosquitto:1883
2026-02-13T14:35:20Z INFO Resubscribed to all topics component=mqtt
```

### 3. Runtime Errors - Data Integrity (Log and Skip)

**Category:** a message whose own content the database can never store. Retrying it cannot succeed, and because `dbWriterLoop` is the single consumer of the buffer, retrying it would stall every message queued behind it. Specified by Epic 5, after the production incident recorded in `docs/sprint-change-proposals/2026-09-17-write-pipeline-stall.md`.

**Handling Strategy: repair what can be repaired, reject the rest, never retry it**

`internal/database` runs these steps in order before the INSERT:

| Step | Condition | Outcome | Log entry |
|------|-----------|---------|-----------|
| 1 | Zero-length payload (MQTT's way of clearing a retained message) | Skipped; not an error | DEBUG `write_skipped`, `reason=empty_payload` |
| 2 | `\u0000` escape, unpaired UTF-16 surrogate escape, invalid UTF-8 byte sequence | Replaced with U+FFFD | WARN `payload_sanitized`, once per message, with one count per repair kind |
| 3 | `json.Valid` fails | Rejected locally, no round-trip | ERROR `write_rejected`, `reason=invalid_json` |
| 4 | INSERT fails with a SQLSTATE of class 22, 23 or 54 | Rejected | ERROR `write_rejected`, `reason=sqlstate`, `sqlstate=<code>` |

Step 2 never touches content PostgreSQL accepts: escaped controls `\u0001`–`\u001F`, noncharacters such as `\uFFFF`, and valid surrogate pairs pass through byte-for-byte. A `\u0000` is repaired only when its backslash starts an escape (preceded by an even number of backslashes): `"\\u0000"` is literal text.

**Write Error Classification:**

| SQLSTATE class | Meaning | Classification | Reason |
|----------------|---------|----------------|--------|
| `22` | Data exception | Permanent | The value is invalid for the column: `22P05` (`\u0000`), `22P02` (invalid JSON, unpaired surrogate), `22021` (invalid UTF-8), `22003` (number out of `numeric` range) |
| `23` | Integrity constraint violation | Permanent | The row breaks a constraint: `23502` (NULL payload). `23505` never surfaces, because `ON CONFLICT DO NOTHING` absorbs duplicates |
| `54` | Program limit exceeded | Permanent | The value exceeds a server limit: `54001` (nesting depth), `54000` (`jsonb` size) |
| `08`, `40`, `53`, `57`, `58`, `XX` | Connection, transaction conflict, resources, operator intervention, system, internal | Transient | They describe the environment, which recovers |
| `42` | Syntax error or access rule violation | Transient | It hits every message identically, and an operator can fix it (a missing `GRANT`, a missing table). Waiting keeps the buffered messages |
| n/a | Not a `*pgconn.PgError` (network error, context deadline) | Transient | No answer came from the database |

```go
// internal/database — callers match the sentinel and never see SQLSTATE codes.
var ErrRejected = errors.New("message rejected by database")

func isPermanent(err error) bool {
    var pgErr *pgconn.PgError
    if !errors.As(err, &pgErr) {
        return false
    }
    switch pgErr.Code[:2] {
    case "22", "23", "54":
        return true
    }
    return false
}

// cmd/mqtt2bdd — insertWithRetry
err := dbClient.InsertMessage(attemptCtx, msg.Topic, msg.Timestamp, json.RawMessage(msg.Payload))
if err == nil || errors.Is(err, database.ErrRejected) {
    return // stored, or never storable: either way the message leaves the buffer now
}
```

**Duplicate Detection (unchanged):**
```go
// InsertMessage checks RowsAffected(): zero means ON CONFLICT (sensor, date) skipped the row.
if commandTag.RowsAffected() == 0 {
    c.logger.Warn("duplicate message ignored", "event", "write_duplicate", ...)
}
// No error returned - idempotent behavior
```

**Example Log Output (illustrative):**
```
level=WARN msg="payload sanitized" component=database event=payload_sanitized topic=zigbee2mqtt/bridge/definitions payload_size=283574 nul_escapes=1 lone_surrogates=0 invalid_utf8_sequences=0
level=ERROR msg="message rejected" component=database event=write_rejected operation=insert topic=sensors/broken payload_size=17 duration_us=0 reason=invalid_json error="message rejected by database: invalid JSON"
level=ERROR msg="message rejected" component=database event=write_rejected operation=insert topic=sensors/huge payload_size=18 duration_us=640 reason=sqlstate sqlstate=22003 error="ERROR: value overflows numeric format (SQLSTATE 22003)"
```

**Rationale:**
- A single bad message must never stall the pipeline: the writer is the buffer's only consumer.
- `write_rejected` is an ERROR, not a WARN: a discarded message is lost data, and an operator filtering on `level=ERROR` must see it.
- The payload body is never logged at ERROR: it can be large (`zigbee2mqtt/bridge/definitions` weighs about 283 KB). `topic` and `payload_size` identify the message; the DEBUG `message_received` entry carries the body when needed.
- U+FFFD rather than deletion keeps each repair visible in the stored data. `jsonb` never stored payloads byte-for-byte anyway: it reorders keys, drops duplicate keys and normalises whitespace.
- A server-side rejection is an answer from the database, so it leaves the client connected. Reporting `db_status=disconnected` would send the operator after the wrong problem.
- **Prerequisite:** the database uses the `UTF8` encoding. Under another encoding, PostgreSQL rejects every non-ASCII escape with `22P05`, and those messages are discarded.

### 4. Runtime Errors - Resource Exhaustion (Backpressure)

**Category:** Channel full, memory limits

**Examples:**
- Message channel full (1000/1000)
- Paho internal buffer full
- Database writer can't keep up with message rate

**Handling Strategy: Backpressure + Logging + Message Dropping (Last Resort)**

**Channel Full Handling with Timeout:**
```go
func (h *MessageHandler) HandleMessage(topic string, payload []byte) {
    msg := Message{
        Topic:     topic,
        Timestamp: time.Now(),
        Payload:   payload,
    }

    // Try to send with 30-second timeout
    select {
    case h.msgChan <- msg:
        // Success
    case <-time.After(30 * time.Second):
        // Channel still full after 30s - drop message
        h.logger.Error("Message dropped: channel full for 30s",
            "topic", topic,
            "buffer_utilization", len(h.msgChan),
            "buffer_capacity", cap(h.msgChan),
        )
        // Increment dropped message counter
        h.droppedMessages.Add(1)
    }
}
```

**Health Check Warning:**
```go
func (app *Application) HealthCheck() {
    ticker := time.NewTicker(60 * time.Second)
    defer ticker.Stop()

    for range ticker.C {
        bufferLen := len(app.msgChan)
        bufferCap := cap(app.msgChan)
        utilization := float64(bufferLen) / float64(bufferCap) * 100

        // Warning if buffer >80% full
        if utilization > 80 {
            app.logger.Warn("Message buffer high utilization",
                "buffer_size", bufferLen,
                "buffer_capacity", bufferCap,
                "utilization_percent", fmt.Sprintf("%.1f", utilization),
            )
        }

        app.logger.Info("Health check",
            "mqtt_connected", app.mqttClient.IsConnected(),
            "buffer_utilization", fmt.Sprintf("%d/%d", bufferLen, bufferCap),
            "dropped_messages_total", app.droppedMessages.Load(),
        )
    }
}
```

**Example Log Output:**
```
2026-02-13T14:50:00Z INFO Health check mqtt_connected=true buffer_utilization=850/1000 dropped_messages_total=0
2026-02-13T14:51:00Z WARN Message buffer high utilization buffer_size=920 buffer_capacity=1000 utilization_percent=92.0
2026-02-13T14:51:30Z ERROR Message dropped: channel full for 30s topic=sensor/high-rate buffer_utilization=1000 buffer_capacity=1000
2026-02-13T14:52:00Z INFO Health check mqtt_connected=true buffer_utilization=1000/1000 dropped_messages_total=5
```

**Rationale:**
- Backpressure (timeout + block) prevents memory exhaustion
- Dropping messages (after 30s timeout) preferable to application crash
- Warnings provide early signal of performance issues
- Metrics (dropped count) visible in logs for monitoring

### 5. Shutdown Errors (Best Effort)

**Category:** Errors during graceful shutdown

**Examples:**
- Failed to flush message to database during shutdown
- Database connection already closed
- MQTT disconnect timeout

**Handling Strategy: Log ERROR and Continue Shutdown**

**Graceful Shutdown Implementation:**

```go
func (app *Application) Shutdown(ctx context.Context) error {
    app.logger.Info("Shutdown signal received")

    // Step 1: Disconnect MQTT (stop receiving new messages)
    if err := app.mqttClient.Disconnect(); err != nil {
        app.logger.Error("MQTT disconnect error (non-fatal)",
            "error", err,
            "component", "mqtt",
        )
        // Continue shutdown anyway
    }
    app.logger.Info("MQTT disconnected")

    // Step 2: Close message channel (signal DB writer to finish)
    close(app.msgChan)
    app.logger.Info("Message channel closed, draining buffer")

    // Step 3: Wait for DB writer goroutine to drain buffer
    app.wg.Wait()  // Blocks until DB writer calls wg.Done()

    flushedCount := app.flushedMessageCount.Load()
    app.logger.Info("Buffer drained",
        "messages_flushed", flushedCount,
    )

    // Step 4: Close database connection
    if err := app.dbClient.Close(); err != nil {
        app.logger.Error("Database close error (non-fatal)",
            "error", err,
            "component", "database",
        )
        // Continue shutdown anyway
    }
    app.logger.Info("Database connection closed")

    app.logger.Info("Shutdown complete")
    return nil  // Always return nil (best effort)
}
```

**Signal Handler (main.go):**

```go
func main() {
    // ... initialization ...

    // Setup signal handler for graceful shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

    // Block until signal received
    sig := <-sigChan
    logger.Info("Received shutdown signal", "signal", sig.String())

    // Execute graceful shutdown (with 30-second timeout)
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := app.Shutdown(shutdownCtx); err != nil {
        logger.Error("Shutdown error", "error", err)
        os.Exit(1)
    }

    os.Exit(0)
}
```

**Example Log Output (Successful Shutdown):**
```
2026-02-13T15:00:00Z INFO Received shutdown signal signal=SIGTERM
2026-02-13T15:00:00Z INFO Shutdown signal received
2026-02-13T15:00:00Z INFO MQTT disconnected
2026-02-13T15:00:00Z INFO Message channel closed, draining buffer
2026-02-13T15:00:01Z DEBUG Message persisted sensor=bedroom/temp timestamp=2026-02-13T14:59:55Z
2026-02-13T15:00:01Z DEBUG Message persisted sensor=living_room/temp timestamp=2026-02-13T14:59:58Z
2026-02-13T15:00:01Z INFO DB writer goroutine exiting (channel closed)
2026-02-13T15:00:01Z INFO Buffer drained messages_flushed=2
2026-02-13T15:00:01Z INFO Database connection closed
2026-02-13T15:00:01Z INFO Shutdown complete
```

**Example Log Output (Shutdown with Errors):**
```
2026-02-13T15:05:00Z INFO Received shutdown signal signal=SIGTERM
2026-02-13T15:05:00Z INFO Shutdown signal received
2026-02-13T15:05:00Z ERROR MQTT disconnect error (non-fatal) error="connection already closed" component=mqtt
2026-02-13T15:05:00Z INFO MQTT disconnected
2026-02-13T15:05:00Z INFO Message channel closed, draining buffer
2026-02-13T15:05:10Z INFO Buffer drained messages_flushed=0
2026-02-13T15:05:10Z ERROR Database close error (non-fatal) error="connection pool already closed" component=database
2026-02-13T15:05:10Z INFO Database connection closed
2026-02-13T15:05:10Z INFO Shutdown complete
```

**Rationale:**
- Graceful shutdown is best-effort
- Errors during shutdown shouldn't prevent process exit
- Log errors for debugging but don't block shutdown
- Container orchestration expects timely exit (SIGKILL after timeout)
- 30-second timeout ensures shutdown doesn't hang indefinitely

**Docker Compose Integration:**

```yaml
services:
  app:
    stop_signal: SIGTERM
    stop_grace_period: 30s  # Time allowed for graceful shutdown before SIGKILL
```

## Error Propagation Strategy

**Layer-Based Error Handling:**

```
┌─────────────────────┐
│   main.go           │  → Fatal errors exit
│   (orchestration)   │  → Runtime errors logged and retried
└──────────┬──────────┘
           │
┌──────────▼──────────┐
│   Component Layer   │  → Return errors to caller with context
│   (mqtt, database)  │  → Wrap errors: fmt.Errorf("...: %w", err)
└──────────┬──────────┘
           │
┌──────────▼──────────┐
│   Library Layer     │  → Return raw errors
│   (Paho, pgx)       │  → Standard Go error values
└─────────────────────┘
```

**Error Wrapping Pattern:**

```go
// Component layer (internal/database/client.go)
func (c *Client) InsertMessage(ctx context.Context, sensor string, ts time.Time, metrics json.RawMessage) error {
    _, err := c.pool.Exec(ctx, query, sensor, ts, metrics)
    if err != nil {
        return fmt.Errorf("database insert failed for sensor %s: %w", sensor, err)
    }
    return nil
}

// Main layer (cmd/mqtt2bdd/main.go)
func dbWriterLoop(msgChan <-chan Message, dbClient *database.Client, logger *slog.Logger) {
    for msg := range msgChan {
        // Retry loop with full error context
        for {
            err := dbClient.InsertMessage(ctx, msg.Topic, msg.Timestamp, msg.Payload)
            if err == nil {
                break
            }

            logger.Warn("Database write failed, retrying in 10s",
                "error", err,  // Includes wrapped context
                "topic", msg.Topic,
            )
            time.Sleep(10 * time.Second)
        }
    }
}
```

**Benefits:**
- Error context preserved through call stack
- Root cause visible in logs
- Debugging easier with full error chain

## Logging Standards for Errors

**Structured Logging Format (slog TextHandler):**

```go
// ERROR level - operation failed, needs attention
logger.Error("Database write failed",
    "error", err,                  // Error message
    "component", "database",       // Which component
    "operation", "insert",         // What operation
    "sensor", sensor,              // Context: what data
    "attempt", attempt,            // Context: retry count
    "duration_ms", elapsed.Milliseconds(),
)

// WARN level - degraded state but recovering
logger.Warn("MQTT connection unstable",
    "error", err,
    "component", "mqtt",
    "reconnect_count", count,
)

// DEBUG level - verbose error details (development)
logger.Debug("Retry scheduled",
    "component", "database",
    "next_attempt_in", "10s",
)
```

**Required Context Fields:**
- `error` - Error message string
- `component` - Package/module name (mqtt, database, main)
- `operation` - What was being attempted (insert, connect, subscribe)
- Domain-specific context - Sensor name, message ID, etc.

**Log Levels:**
- **DEBUG:** Retry attempts, transient errors (noisy in production)
- **WARN:** Degraded state, duplicates, buffer high
- **ERROR:** Failed operations, connection failures
- **FATAL:** Unrecoverable errors (startup only)

The enumerated values these fields may take — every `component`, `event` and `operation` in use — are documented in [Logging Standards](./logging-standards.md).

## Operational Error Response Playbook

**Error Response Matrix:**

| Error Log | Severity | Action | Timeline |
|-----------|----------|--------|----------|
| `Configuration error` at startup | FATAL | Fix env vars, restart container | Immediate |
| `MQTT connection lost` | ERROR | Automatic retry (10s interval) | Wait 1 min, check broker if persists |
| `Database connection lost` | ERROR | Automatic retry (10s interval) | Wait 1 min, check PostgreSQL if persists |
| `Message buffer high utilization` (>80%) | WARN | Check database performance, buffer draining | Investigate within 10 min |
| `Duplicate message ignored` | WARN | Normal (idempotent), no action needed | Monitor frequency |
| `Message dropped: channel full` | ERROR | Database slow or down, investigate immediately | < 5 min |
| `Invalid JSON payload` | WARN | Check device/broker, investigate data source | Non-urgent |

**Escalation Path:**
1. **Self-healing** (0-5 min): Automatic retries handle transient failures
2. **Monitoring alert** (5-15 min): If errors persist, alert operator
3. **Manual intervention** (15+ min): Investigate root cause, restart services if needed

**Common Resolution Steps:**

```bash
# Check application logs
docker logs mqtt2bdd-prod-app --tail 100

# Check MQTT broker connectivity
docker exec mqtt2bdd-prod-app ping -c 3 mosquitto.local

# Check PostgreSQL connectivity
docker exec mqtt2bdd-prod-app psql -h postgres.local -U mqtt2bdd -c "SELECT 1;"

# Restart application (if self-healing fails)
docker-compose -f docker-compose.prod.yml restart app

# Check resource usage
docker stats mqtt2bdd-prod-app
```

## Error Handling Strategy Summary

MQTT2BDD's error handling emphasizes **resilience through automatic recovery** while maintaining **operational visibility through comprehensive logging**. The strategy balances:

- **Strictness:** Fail-fast at startup for misconfigurations
- **Leniency:** Automatic infinite retry for transient network issues
- **Safety:** Backpressure and message dropping (last resort) over crashes
- **Isolation:** A message the database can never accept is repaired or discarded, never retried, so it cannot stall the messages behind it
- **Observability:** Structured logs with full context for debugging

All error handling follows Go idioms (explicit errors, no panics) and supports the project's educational objective by demonstrating production-grade error patterns without over-engineering.

---
