# MQTT2BDD Product Requirements Document (PRD)

## Goals and Background Context

### Goals

- Learn Go fundamentals through building a production-quality application (concurrency, interfaces, error handling, package structure)
- Capture 99%+ of all MQTT home automation messages from Zigbee2MQTT devices into PostgreSQL for persistent storage
- Enable historical analysis and Grafana visualization of temperature trends, energy consumption patterns, and device behavior
- Build autonomous, zero-maintenance service with automatic recovery from MQTT/database connection failures
- Create educational codebase demonstrating Go best practices transferable to professional development contexts
- Deploy via Docker on Alpine Linux with environment-based configuration requiring no code changes
- Achieve >80% test coverage with comprehensive unit tests demonstrating Go testing patterns

### Background Context

Home automation systems like Zigbee2MQTT publish device state changes and sensor readings via MQTT in real-time, but these notifications are ephemeral—once consumed or missed, the data is lost forever. With approximately 8 thermostats currently deployed and plans to scale to 100+ connected devices (switches, gas/electricity meters, sensors), valuable operational data is continuously lost, preventing energy optimization, predictive maintenance, debugging of intermittent issues, and data-driven decision-making.

MQTT2BDD bridges this gap as a standalone Go application that subscribes to all MQTT topics using a wildcard subscription, capturing each message's topic, timestamp, and JSON payload into a PostgreSQL `sensor_metrics` table. The application serves dual purposes: solving a real infrastructure need for home automation persistence while providing a learning-oriented codebase that demonstrates production-grade Go patterns including connection pooling, graceful shutdown, structured logging, and resilient error handling—all deployable in a minimal Docker container on Alpine Linux 3.23.

### Change Log

| Date       | Version | Description                                                  | Author          |
|------------|---------|--------------------------------------------------------------|-----------------|
| 2026-01-23 | 0.1.0   | Initial PRD creation from Project Brief                      | PM Agent (John) |
| 2026-07-25 | 0.2.0   | Epic 2 alignment - Story 2.5 AC9/AC10, Story 2.6 AC2/AC4/AC7 | PO (Sarah)      |
| 2026-07-27 | 0.3.0   | Epic 4 added - module rename + public binary distribution    | PO (Sarah)      |
| 2026-07-29 | 0.3.1   | Story 4.1 AC2/AC3 file counts realigned (8 -> 9 Go files, 18 -> 19 story files) | PO (Sarah)      |
| 2026-09-17 | 0.4.0   | Epic 5 added - write pipeline hardening; FR1, FR6, FR8 amended; FR11, FR12 added | PO (Sarah)      |

Amendments are recorded inline, in a blockquote beneath the acceptance criteria of the story
they affect, so a reader arriving at an AC always finds the reason it reads as it does.

---

## Requirements

### Functional Requirements

1. **FR1:** The application must subscribe to all MQTT topics using wildcard (`#`) on a configured MQTT broker and receive messages in real-time, discarding before buffering any message whose topic matches an optional, operator-supplied list of MQTT topic filters
2. **FR2:** The application must parse each MQTT message to extract topic (sensor identifier), timestamp, and JSON payload
3. **FR3:** The application must insert normalized records into PostgreSQL `sensor_metrics` table with columns: sensor (VARCHAR), date (TIMESTAMP), metrics (JSONB)
4. **FR4:** The application must detect MQTT broker disconnections and automatically reconnect with fixed 10-second retry interval
5. **FR5:** The application must detect PostgreSQL database disconnections and automatically reconnect with fixed 10-second retry interval
6. **FR6:** The application must buffer incoming MQTT messages in memory (capacity: 1000 messages) to handle temporary database outages without message loss. This guarantee covers failures of the database, not of a message: a payload the database can never accept is handled by FR11
7. **FR7:** The application must handle SIGTERM/SIGINT signals gracefully, flushing all buffered messages to database before shutdown
8. **FR8:** The application must accept configuration exclusively via environment variables (MQTT host/port/credentials, PostgreSQL connection string, log level, buffer size, topic exclusion list)
9. **FR9:** The application must implement structured logging with levels (INFO, DEBUG, ERROR) for startup, shutdown, connection events, message processing, and errors
10. **FR10:** The application must log all critical lifecycle events including: application start/stop, MQTT connection established/lost, database connection established/lost, and write failures with context
11. **FR11:** A single message must never stall the pipeline. A payload the database can never accept — one that is not valid JSON, or one the database rejects with a SQLSTATE of class 22 (data exception), 23 (integrity constraint violation) or 54 (program limit exceeded) — must be discarded immediately and logged at ERROR. Every other write failure keeps being retried as per FR5. An empty payload, which MQTT uses to clear a retained message, is skipped without being treated as an error
12. **FR12:** Before insertion, the application must repair the content PostgreSQL would refuse but that can be repaired without guesswork, replacing it with the Unicode replacement character U+FFFD: the JSON escape `\u0000`, unpaired UTF-16 surrogate escapes, and invalid UTF-8 byte sequences. Each repaired message is logged at WARN

### Non-Functional Requirements

1. **NFR1:** The application must run reliably with automatic recovery from MQTT broker and database connection failures
2. **NFR2:** The application must be deployable as a Docker container based on Alpine Linux 3.23 with a statically-linked Go binary
3. **NFR3:** The application must have comprehensive unit tests demonstrating Go testing best practices for core components (MQTT handler, database writer, reconnection logic)
4. **NFR4:** All code must pass `go fmt`, `go vet`, and `staticcheck` checks, following Go effective conventions and idiomatic patterns
5. **NFR5:** The codebase must follow Go project layout best practices with clear package separation (`cmd/`, `internal/`, `pkg/`)
6. **NFR6:** The application must include comprehensive documentation (README) with architecture overview, setup instructions, configuration reference, and code walkthrough for Go learners
7. **NFR7:** The application must handle current load (8 thermostats) with sufficient headroom for planned expansion to 100+ devices
8. **NFR8:** All error messages must be clear, actionable, and include sufficient context (what failed, why, and suggested remediation)
9. **NFR9:** The Docker image must use multi-stage builds on Alpine Linux for minimal footprint

---

## Technical Assumptions

### Repository Structure: Monorepo

Single repository following Go project layout best practices:
- `cmd/mqtt2bdd/` - Application entry point (main.go)
- `internal/` - Private application packages (mqtt, database, config, logger)
- `pkg/` - Public libraries (if any reusable components emerge)
- `dev/` - Development Docker Compose stack (PostgreSQL + Mosquitto)
- `test/` - Test Docker Compose stack for isolated integration testing

**Rationale:** Single application doesn't require polyrepo complexity. Go project layout provides clear separation of concerns while keeping everything in one place for learning purposes.

### Service Architecture: Monolith

Single-process Go application with goroutine-based concurrency:
- Main goroutine: Application lifecycle management, signal handling
- MQTT goroutine: Message reception and channel publishing
- Database writer goroutine: Consumes from buffered channel, writes to PostgreSQL
- No microservices, no separate services, no inter-process communication

**Rationale:** MQTT2BDD has a single, focused responsibility (MQTT → PostgreSQL bridge). Microservices would add unnecessary complexity, deployment overhead, and violate the learning objective of understanding Go concurrency within a single process.

### Testing Requirements: Comprehensive Unit + Integration Tests

- **Unit tests:** Core components (message parsing, database operations, reconnection logic) with table-driven tests demonstrating Go patterns
- **Integration tests:** End-to-end MQTT → PostgreSQL flow using test Docker Compose stack
- **No UI tests:** Application has no user interface
- **Coverage focus:** Core business logic and error handling paths, not trivial code
- **Test infrastructure:** Separate `test/docker-compose.yml` providing isolated PostgreSQL + Mosquitto instances

**Rationale:** Learning objective requires comprehensive testing examples. Integration tests validate real-world behavior against actual MQTT broker and database. Isolated test environment prevents pollution of development data.

### Additional Technical Assumptions and Requests

- **Language & Runtime:** Go 1.23+ (latest stable version), compiled as statically-linked binary for Alpine Linux compatibility
- **MQTT Client Library:** `github.com/eclipse/paho.mqtt.golang` - Official Eclipse Paho client, widely used, well-documented, production-ready
- **PostgreSQL Driver:** `github.com/jackc/pgx/v5` - Modern, high-performance driver with excellent error handling and connection pooling support
- **Logging Framework:** Standard library `log/slog` (Go 1.21+, available in 1.23) for structured logging with levels (INFO, DEBUG, ERROR)
- **Configuration Management:** Standard library `os.Getenv()` for environment variable handling - zero dependencies, educational value
- **Container Base Image:** Alpine Linux 3.23 - minimal footprint, widely used for Go applications
- **Build Process:** Multi-stage Dockerfile (builder stage with Go toolchain, runtime stage with Alpine + binary only)
- **Development Environment (Fully Containerized):** Three-container setup via `dev/docker-compose.yml`:
  - PostgreSQL container (`mqtt2bdd-dev-postgres`) for database
  - Mosquitto container (`mqtt2bdd-dev-mosquitto`) for MQTT broker
  - Go development container (`mqtt2bdd-dev-go-dev`) with volume-mounted project directory for live code editing
  - **Container naming:** All containers prefixed with `mqtt2bdd-dev-` for easy identification among other projects
  - **Delve debugger integration:** Go container configured with Delve (dlv) for remote debugging via exposed port (equivalent to XDebug workflow familiar to PHP developers)
  - One-command startup: `docker-compose up` in `dev/` directory
  - Hot reload capability for rapid development iteration
- **Database Schema:** Pre-existing `sensor_metrics` table with schema: `sensor VARCHAR, date TIMESTAMP, metrics JSONB` (application does NOT create schema, assumes it exists)
- **Deployment Model:** Single Docker container, configured entirely via environment variables, suitable for Docker Compose or Kubernetes deployment
- **Concurrency Model:** Goroutines + channels for concurrent MQTT message handling and database writes, demonstrating Go's CSP (Communicating Sequential Processes) paradigm
- **Error Handling Philosophy:** Explicit error returns following Go conventions, no exceptions/panics in normal operation, all errors logged with context
- **Dependency Management:** Go modules (`go.mod`) with minimal external dependencies (only Paho MQTT client and pgx driver)
- **Code Quality Tools:** `go fmt` (formatting), `go vet` (static analysis), and `staticcheck` (advanced linter) - `staticcheck` is the modern industry-standard replacement for deprecated `golint`
- **Application Versioning:** Version injected at build time via ldflags (`-X main.Version=<version>`), following Go industry best practices - allows automated versioning in CI/CD without code modification

---

## Epic List

### Epic 1: Containerized Dev Environment & Core MQTT-to-PostgreSQL Pipeline

Establish fully containerized development environment with three containers (PostgreSQL, Mosquitto, Go dev environment with Delve debugging support), initialize Go project structure following best practices, and deliver a working MQTT subscription handler that captures messages and writes them to PostgreSQL.

### Epic 2: Production Resilience & Reliability

Implement automatic reconnection logic for MQTT and database failures, message buffering, graceful shutdown handling, and comprehensive structured logging to create an autonomous, self-healing service.

### Epic 3: Production Docker Image & Educational Documentation

Create optimized production Dockerfile (multi-stage Alpine build distinct from dev container), establish integration testing infrastructure with isolated test docker-compose stack, and write comprehensive educational documentation covering architecture, deployment, debugging setup, and Go patterns walkthrough.

### Epic 4: Public Release & Binary Distribution

Publish the repository on GitHub under an MIT license and automate the production of downloadable, statically-linked binaries for four platforms on every version tag, so that anyone can install MQTT2BDD with a single `curl` command without cloning the repository or installing a Go toolchain.

### Epic 5: Write Pipeline Hardening

Stop a single invalid message from stalling persistence: classify database write errors into transient and permanent, repair the payload content PostgreSQL refuses when that can be done without guesswork, and let operators exclude topics that carry configuration rather than measurements.

---

## Epic 1: Containerized Dev Environment & Core MQTT-to-PostgreSQL Pipeline

**Epic Goal:** Establish a fully functional development environment where developers can start contributing immediately with one command, and deliver the core value proposition—MQTT messages flowing into PostgreSQL—demonstrating the fundamental data pipeline that solves the home automation persistence problem.

### Story 1.1: Setup Containerized Development Environment

**As a** developer learning Go,
**I want** a one-command development environment setup with PostgreSQL, Mosquitto, and Go containers,
**so that** I can start coding immediately without installing any infrastructure locally.

**Acceptance Criteria:**

1. `dev/docker-compose.yml` defines three services: `postgres`, `mosquitto`, and `go-dev`
2. All containers use `container_name` directive with `mqtt2bdd-dev-` prefix: `mqtt2bdd-dev-postgres`, `mqtt2bdd-dev-mosquitto`, `mqtt2bdd-dev-go-dev` for easy identification among other projects
3. PostgreSQL container uses official `postgres:15-alpine` image with environment variables for database name, user, and password
4. Mosquitto container uses official `eclipse-mosquitto:2` image with basic configuration allowing anonymous connections
5. Go development container based on `golang:1.23-alpine` (latest stable) with Delve debugger and `staticcheck` linter installed
6. Go container exposes port 2345 for Delve remote debugging
7. Go container mounts project root directory as volume (e.g., `.:/app`) for live code editing
8. SQL initialization script `dev/init-db/01-schema.sql` creates `sensor_metrics` table with schema: `sensor VARCHAR(255), date TIMESTAMP, metrics JSONB`
9. PostgreSQL container automatically executes initialization scripts on first startup
10. All containers start successfully with `docker-compose up` from `dev/` directory
11. Developer can verify PostgreSQL connectivity: `docker exec -it mqtt2bdd-dev-postgres psql -U <user> -d <dbname> -c "\dt"` shows `sensor_metrics` table
12. Developer can verify Mosquitto connectivity: `docker exec -it mqtt2bdd-dev-mosquitto mosquitto_sub -t '#' -v` listens to all topics
13. `.gitignore` includes `.env` file and Docker-related temporary files
14. `dev/.env.example` provides template for required environment variables with documentation
15. `.vscode/launch.json` configuration provided for remote debugging (connects to `localhost:2345`)
16. Manual debugging test: Set breakpoint in VS Code (or compatible IDE), attach debugger to port 2345, run application in container, verify breakpoint is hit and variables can be inspected

### Story 1.2: Initialize Go Project Structure

**As a** Go learner,
**I want** a project structure following Go best practices,
**so that** I understand idiomatic project organization and can navigate the codebase easily.

**Acceptance Criteria:**

1. Go module initialized with `go mod init github.com/<username>/mqtt2bdd` (or appropriate module path)
2. Directory structure created:
   - `cmd/mqtt2bdd/` (application entry point)
   - `internal/config/` (configuration package)
   - `internal/logger/` (logging package)
   - `internal/mqtt/` (MQTT client package)
   - `internal/database/` (PostgreSQL client package)
   - `pkg/` (public packages, empty for now)
3. `cmd/mqtt2bdd/main.go` contains minimal main function that prints "MQTT2BDD starting..." and exits cleanly
4. Application can be built inside Go dev container: `go build -o bin/mqtt2bdd ./cmd/mqtt2bdd`
5. Application can be run inside Go dev container: `./bin/mqtt2bdd` prints startup message
6. `go.mod` and `go.sum` files tracked in version control
7. All code passes `go fmt`, `go vet`, and `staticcheck` with zero warnings
8. README.md created with sections: Overview, Project Structure, Getting Started (placeholder content)

### Story 1.3: Implement Configuration Package

**As a** developer,
**I want** a centralized configuration package that loads settings from environment variables,
**so that** the application can be configured without code changes.

**Acceptance Criteria:**

1. `internal/config/config.go` defines `Config` struct with fields: `MQTTBroker`, `MQTTPort`, `MQTTUsername`, `MQTTPassword`, `PostgresHost`, `PostgresPort`, `PostgresDB`, `PostgresUser`, `PostgresPassword`, `LogLevel`
2. `LoadConfig()` function reads environment variables using `os.Getenv()` and populates `Config` struct
3. Required environment variables: `MQTT_BROKER`, `MQTT_PORT`, `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`
4. Optional environment variables with defaults: `MQTT_USERNAME` (default: ""), `MQTT_PASSWORD` (default: ""), `LOG_LEVEL` (default: "INFO")
5. `LoadConfig()` returns error if any required environment variable is missing with clear message indicating which variable
6. Unit tests in `internal/config/config_test.go` validate loading with all variables set, with missing required variables, and with default values
7. Unit tests use table-driven test pattern demonstrating Go testing idioms
8. `main.go` updated to call `config.LoadConfig()` on startup and log fatal error if configuration fails
9. All code passes `go fmt`, `go vet`, and `staticcheck`

### Story 1.4: Implement Structured Logging Package

**As a** developer,
**I want** structured logging with configurable levels (INFO, DEBUG, ERROR),
**so that** I can troubleshoot issues effectively in development and production.

**Acceptance Criteria:**

1. `internal/logger/logger.go` wraps Go's `log/slog` package providing `Info()`, `Debug()`, `Error()` methods
2. `InitLogger(level string)` function initializes slog with JSON handler for structured output
3. Log level configurable via parameter: "DEBUG", "INFO", "ERROR" (case-insensitive)
4. Invalid log level defaults to INFO with warning message
5. Logger provides context-aware methods accepting key-value pairs: `logger.Info("message", "key1", value1, "key2", value2)`
6. `main.go` updated to initialize logger with `config.LogLevel` after configuration loads
7. Logger outputs to stdout in JSON format with fields: timestamp, level, message, and any additional context
8. Unit tests validate logger initialization with different levels and output formatting
9. `main.go` declares version variable `var Version = "dev"` and logs application startup at INFO level with version: `logger.Info("MQTT2BDD starting", "version", Version)`
10. All code passes `go fmt`, `go vet`, and `staticcheck`

### Story 1.5: Implement MQTT Client Connection

**As a** developer,
**I want** to establish a connection to the MQTT broker using configuration from environment variables,
**so that** I can subscribe to topics and receive messages.

**Acceptance Criteria:**

1. `internal/mqtt/client.go` defines `Client` struct encapsulating Paho MQTT client
2. `go.mod` includes dependency: `github.com/eclipse/paho.mqtt.golang`
3. `NewClient(config)` function creates MQTT client with broker URL constructed from config (e.g., `tcp://mosquitto:1883`)
4. `Connect()` method establishes connection to MQTT broker with optional username/password authentication if provided
5. Connection uses QoS 0 for simplicity (at-most-once delivery)
6. `Connect()` logs connection attempt (INFO) and successful connection (INFO) or failure (ERROR) with context
7. `Disconnect()` method cleanly closes MQTT connection
8. `main.go` updated to create MQTT client, connect on startup, defer disconnect on exit
9. Manual test: Run application with `docker-compose up`, verify logs show successful MQTT connection to `mqtt2bdd-dev-mosquitto` container
10. Application exits gracefully if MQTT connection fails with clear error message
11. All code passes `go fmt`, `go vet`, and `staticcheck`

### Story 1.6: Implement MQTT Subscription and Message Reception

**As a** developer,
**I want** to subscribe to all MQTT topics and receive messages in real-time,
**so that** I can capture home automation device notifications.

**Acceptance Criteria:**

1. `internal/mqtt/client.go` adds `Subscribe(topic string, handler MessageHandler)` method
2. `MessageHandler` type defined as callback function: `type MessageHandler func(topic string, payload []byte)`
3. `Subscribe()` subscribes to wildcard topic `#` with QoS 0
4. `Subscribe()` logs subscription attempt (INFO) and success/failure with topic name
5. Message handler callback invoked for each received message with topic and payload
6. `main.go` implements simple message handler that logs received messages: topic, payload length, and first 100 bytes of payload (DEBUG level)
7. Manual test: Use `mosquitto_pub` from mqtt2bdd-dev-mosquitto container to publish test message: `docker exec mqtt2bdd-dev-mosquitto mosquitto_pub -t 'test/topic' -m '{"temp": 20.5}'`
8. Verify application logs show received message with correct topic and payload preview
9. Message reception runs in separate goroutine (non-blocking)
10. All code passes `go fmt`, `go vet`, and `staticcheck`

### Story 1.7: Implement PostgreSQL Client Connection

**As a** developer,
**I want** to establish a connection to PostgreSQL database with connection pooling,
**so that** I can persist MQTT messages efficiently.

**Acceptance Criteria:**

1. `internal/database/client.go` defines `Client` struct wrapping pgx connection pool
2. `go.mod` includes dependency: `github.com/jackc/pgx/v5` and `github.com/jackc/pgx/v5/pgxpool`
3. `NewClient(config)` creates connection pool using DSN constructed from config: `postgres://user:pass@host:port/dbname`
4. `Connect(ctx context.Context)` method establishes connection pool with context support
5. Connection pool configuration: min connections = 1, max connections = 5 (sufficient for expected load, prevents resource waste)
6. `Connect()` logs connection attempt (INFO) and success/failure (ERROR) with database host and name
8. `Close()` method cleanly closes connection pool
9. `main.go` updated to create database client, connect on startup, defer close on exit
10. Manual test: Run application, verify logs show successful PostgreSQL connection to `mqtt2bdd-dev-postgres` container
11. Application exits gracefully if database connection fails with clear error message including connection details
12. All code passes `go fmt`, `go vet`, and `staticcheck`

### Story 1.8: Implement PostgreSQL Message Writer

**As a** developer,
**I want** to insert MQTT messages into the sensor_metrics table,
**so that** home automation data is persisted for future analysis.

**Acceptance Criteria:**

1. `internal/database/client.go` adds `InsertMessage(ctx context.Context, sensor string, timestamp time.Time, metrics json.RawMessage)` method
2. `InsertMessage()` executes SQL: `INSERT INTO sensor_metrics (sensor, date, metrics) VALUES ($1, $2, $3)`
3. Method uses parameterized query to prevent SQL injection
4. Method returns error if insert fails, nil on success
5. Logs successful insert (DEBUG) with sensor name, timestamp, and payload size
6. Logs failed insert (ERROR) with full error context including sensor name and error message
7. Unit test (using test database or mocks) validates successful insert and error handling
8. Manual test: Call `InsertMessage()` directly from `main.go` with hardcoded test data on startup
9. Verify test record appears in PostgreSQL: `docker exec -it mqtt2bdd-postgres psql -U <user> -d <dbname> -c "SELECT * FROM sensor_metrics;"`
10. All code passes `go fmt`, `go vet`, and `staticcheck`

### Story 1.9: Integrate MQTT Reception with PostgreSQL Persistence

**As a** home automation user,
**I want** every MQTT message automatically persisted to PostgreSQL,
**so that** I can analyze historical device data.

**Acceptance Criteria:**

1. `main.go` implements message handler that calls `database.InsertMessage()` for each received MQTT message
2. Message handler extracts: topic as sensor identifier, current timestamp, and raw JSON payload
3. Message handler runs asynchronously (goroutine) to avoid blocking MQTT message reception
4. Successful persistence logged at DEBUG level: "Message persisted: topic=<topic>, size=<bytes>"
5. Failed persistence logged at ERROR level with full context but does not crash application
6. Manual end-to-end test: Publish MQTT message from mqtt2bdd-dev-mosquitto container: `docker exec mqtt2bdd-dev-mosquitto mosquitto_pub -t 'zigbee2mqtt/living_room/thermostat' -m '{"temperature": 21.3, "humidity": 45}'`
7. Verify message appears in PostgreSQL sensor_metrics table with correct topic, timestamp, and JSON payload
8. Manual load test: Publish 100 messages rapidly, verify all messages persisted correctly
9. Application startup logs show clear sequence: Config loaded → Logger initialized → MQTT connected → PostgreSQL connected → Subscribed to topics
10. All code passes `go fmt`, `go vet`, and `staticcheck`

---

## Epic 2: Production Resilience & Reliability

**Epic Goal:** Transform the fragile but functional MQTT→PostgreSQL pipeline from Epic 1 into a production-ready, self-healing service that automatically recovers from failures, buffers messages during outages, and provides comprehensive operational visibility through structured logging.

### Story 2.1: Implement Message Buffering with Channels

**As a** system operator,
**I want** incoming MQTT messages buffered in memory,
**so that** temporary database outages don't result in message loss.

**Acceptance Criteria:**

1. `main.go` creates buffered channel with capacity 1000: `msgChan := make(chan Message, 1000)`
2. `Message` struct defined with fields: `Topic string`, `Timestamp time.Time`, `Payload []byte`
3. MQTT message handler (from Story 1.9) sends messages to channel instead of directly inserting to database
4. Separate database writer goroutine consumes messages from channel and calls `database.InsertMessage()`
5. If channel is full (1000 messages), MQTT handler logs WARNING and blocks until space is available
6. Database writer goroutine processes messages sequentially from channel
7. Successful writes logged at DEBUG level, failures at ERROR level
8. Manual test: Stop PostgreSQL container, publish 50 MQTT messages, restart PostgreSQL, verify all 50 messages eventually written
9. Channel and goroutine architecture clearly documented in code comments demonstrating Go CSP pattern
10. All code passes `go fmt`, `go vet`, and `staticcheck`

### Story 2.2: Implement MQTT Automatic Reconnection

**As a** system operator,
**I want** the application to automatically reconnect to MQTT broker after connection loss,
**so that** the service recovers without manual intervention.

**Acceptance Criteria:**

1. `internal/mqtt/client.go` adds connection lost callback handler to MQTT client configuration
2. On connection lost, log ERROR with reason: "MQTT connection lost: <reason>"
3. Implement reconnection loop with fixed 10-second retry interval
4. Reconnection attempts logged at INFO level: "Attempting MQTT reconnection (attempt <n>)..."
5. On successful reconnection, log INFO: "MQTT reconnected successfully" and re-subscribe to topics
6. Reconnection runs in separate goroutine to avoid blocking main application
7. Reconnection loop continues indefinitely until connection is re-established
8. Application continues running during MQTT outage (doesn't crash or exit)
9. Manual test: Stop Mosquitto container, verify reconnection attempts logged every 10 seconds, restart Mosquitto, verify successful reconnection and message reception resumes
10. All code passes `go fmt`, `go vet`, and `staticcheck`

### Story 2.3: Implement PostgreSQL Automatic Reconnection

**As a** system operator,
**I want** the application to automatically reconnect to PostgreSQL after connection loss,
**so that** database writes resume without manual intervention.

**Acceptance Criteria:**

1. `internal/database/client.go` wraps all database operations with connection health checks
2. On write failure due to connection error, log ERROR: "Database connection lost: <error>"
3. Implement reconnection loop with fixed 10-second retry interval
4. Reconnection attempts logged at INFO level: "Attempting database reconnection (attempt <n>)..."
5. Retry failed INSERT operations with 10-second interval until successful (connection verification implicit in successful INSERT)
6. On successful write after reconnection, log INFO: "Database reconnected successfully"
7. Failed messages during outage remain in channel buffer (not lost) and are retried after reconnection
8. Database writer goroutine continues processing buffered messages after reconnection
9. Manual test: Stop PostgreSQL container, publish MQTT messages (buffered), restart PostgreSQL, verify all buffered messages written after reconnection
10. All code passes `go fmt`, `go vet`, and `staticcheck`

### Story 2.4: Implement Graceful Shutdown Handler

**As a** system operator,
**I want** the application to flush all buffered messages before shutdown,
**so that** no data is lost when the container is stopped.

**Acceptance Criteria:**

1. `main.go` registers signal handler for SIGTERM and SIGINT using `signal.Notify()`
2. On receiving shutdown signal, log INFO: "Shutdown signal received, flushing buffered messages..."
3. Close MQTT client connection to stop receiving new messages
4. Close message channel to signal database writer goroutine to finish processing
5. Database writer goroutine processes all remaining messages in channel before exiting
6. Wait for database writer goroutine to complete using `sync.WaitGroup` or channel completion signal
7. Log INFO with count of messages flushed: "Flushed <n> buffered messages"
8. Close database connection pool after all messages written
9. Log INFO: "Shutdown complete" and exit with code 0
10. Manual test: Publish messages, send SIGTERM (`docker stop <container>`), verify all buffered messages written before container exits
11. Maximum shutdown time: 30 seconds (configurable via Docker stop timeout)
12. All code passes `go fmt`, `go vet`, and `staticcheck`

### Story 2.5: Enhance Structured Logging for Operations

**As a** system operator,
**I want** comprehensive structured logs with consistent context fields,
**so that** I can troubleshoot issues and monitor application health.

**Acceptance Criteria:**

1. All log entries include consistent context fields: `timestamp`, `level`, `message`, `component` (e.g., "mqtt", "database", "main")
2. MQTT events logged with context: `event` (connected/disconnected/subscribed), `broker`, `topic` (if applicable)
3. Database events logged with context: `event` (connected/disconnected/write_success/write_failure), `host`, `database`
4. Message processing logged with context: `topic`, `payload_size`, `duration_ms` (time to write)
5. Error logs include full error context: `error`, `component`, `operation` (what was being attempted)
6. Startup sequence logged showing initialization of each component
7. Log output includes application version injected via build-time ldflags (e.g., `-ldflags="-X main.Version=0.1.0"`), defaulting to "dev" for local development
8. All timestamps in ISO 8601 format with timezone (UTC)
9. Debug mode logs additional details: raw MQTT payloads in full (no truncation), SQL queries, connection pool stats
10. Manual test: Review logs in Docker logs output, verify structured-field consistency across components
11. All code passes `go fmt`, `go vet`, and `staticcheck`

> **Amended 2026-07-25 (PO), after implementation.** AC9 and AC10 originally read "raw MQTT payloads (truncated)" and "verify JSON structure". Both were deliberately diverged from during implementation and the divergences were accepted at review; the wording above is what shipped.
>
> - **AC9** — payloads are logged whole. A cap removes the end of the payload, which is as often as not where the evidence sits when a message fails to parse. DEBUG is opt-in, so verbosity stays bounded by the operator's own `LOG_LEVEL`.
> - **AC10** — the logger keeps `slog.NewTextHandler`. Text output is mandated by three architecture documents and has shipped since Story 1.4; the AC's intent (an operator can read and correlate fields) is met by `key=value`. Genuine JSON output would be an architecture change and warrants its own story.
>
> [Source: docs/stories/2.5.story.md → Dev Notes → Deviations; docs/qa/gates/2.5-enhance-structured-logging-for-operations.yml]

### Story 2.6: Add Application Health Monitoring Logs

**As a** system operator,
**I want** periodic health status logs,
**so that** I can verify the application is running correctly.

**Acceptance Criteria:**

1. Implement periodic health check goroutine running every 60 seconds
2. Health check logs at INFO level as structured fields rather than an interpolated sentence: `event=health_check`, `mqtt_status`, `db_status`, `db_last_write_age_s`, `buffer_used`, `buffer_capacity`, `buffer_utilization_percent`, `processed_last_interval`
3. MQTT connection status verified using client `IsConnected()` method
4. Database connection status derived from the outcome of the most recent write, seeded by the startup `Ping` — no explicit `Ping` in the health check. Write recency is reported alongside it as its own field, `db_last_write_age_s`
5. Buffer utilization calculated from channel length
6. If buffer utilization >80%, log WARNING: "Message buffer >80% full, possible backpressure"
7. Health check includes the message processing rate as a `processed_last_interval` field on the same entry: the number of messages written since the previous tick
8. Health check goroutine uses ticker for precise intervals
9. Manual test: Monitor logs for 5 minutes, verify health checks appear every 60 seconds with accurate status
10. All code passes `go fmt`, `go vet`, and `staticcheck`

> **Amended 2026-07-25 (PO), before implementation.** AC2, AC4 and AC7 originally prescribed interpolated log sentences (`"Health check: MQTT=..., DB=..., Buffer=n/capacity messages"`, `"Processed n messages in last 60s"`) and a recency test for the database status. The wording above replaces them.
>
> - **AC2 and AC7** — the values become structured attributes and `msg` stays a constant phrase, per the convention Story 2.5 established and the health-check example the architecture already gives. Interpolating the numbers would make the one entry an operator most wants to aggregate the only entry they cannot chart.
> - **AC4** — a literal recency rule ("a write succeeded in the last 60 s ⇒ connected") reports `disconnected` for a perfectly healthy database whenever no message happened to arrive, and quiet minutes are normal on a home-automation broker. Deriving the status from the *outcome* of the last write honours AC4's real constraint — no `Ping`, a single atomic load, no network call — while dropping only the part of the wording that would produce false alarms. Recency remains reported, as `db_last_write_age_s`.
>
> [Source: docs/stories/2.6.story.md → Dev Notes → Deviations]

---

## Epic 3: Production Docker Image & Educational Documentation

**Epic Goal:** Package the production-ready application into an optimized Docker container, establish comprehensive testing infrastructure, and create educational documentation that enables Go learners to understand the architecture, deploy the application, and leverage the codebase as a learning resource.

### Story 3.1: Create Production Multi-Stage Dockerfile

**As a** DevOps engineer,
**I want** an optimized production Docker image,
**so that** the application can be deployed efficiently with minimal resource footprint.

**Acceptance Criteria:**

1. `Dockerfile` (root directory) implements multi-stage build with two stages: `builder` and `runtime`
2. Builder stage uses `golang:1.23-alpine` image with full Go toolchain
3. Builder stage defines `ARG VERSION=dev` for version injection at build time
4. Builder stage copies source code, runs `go mod download`, and compiles binary: `go build -ldflags="-s -w -X main.Version=${VERSION}" -o /app/mqtt2bdd ./cmd/mqtt2bdd`
5. `cmd/mqtt2bdd/main.go` declares version variable: `var Version = "dev"` (default for local development, overridden by ldflags at build)
6. Builder stage produces statically-linked binary (no dynamic dependencies)
7. Runtime stage uses `alpine:3.23` base image (minimal, no Go toolchain)
8. Runtime stage installs only CA certificates (`ca-certificates` package) for TLS support
9. Runtime stage copies compiled binary from builder stage
10. Runtime stage sets non-root user for security: `USER nobody`
11. Final image size optimized (target <20MB)
12. Dockerfile includes labels: version, description, maintainer
13. `ENTRYPOINT ["/app/mqtt2bdd"]` configured for container execution
14. Manual test: Build image with version: `docker build --build-arg VERSION=0.1.0 -t mqtt2bdd:0.1.0 .`, verify application logs show correct version on startup
15. All code passes `go fmt`, `go vet`, and `staticcheck`

### Story 3.2: Create Production Docker Compose Configuration

**As a** DevOps engineer,
**I want** a production-ready Docker Compose configuration,
**so that** the full stack (app, PostgreSQL, MQTT) can be deployed with one command.

**Acceptance Criteria:**

1. `docker-compose.prod.yml` (root directory) defines three services: `postgres`, `mosquitto`, `mqtt2bdd`
2. PostgreSQL service uses official `postgres:15-alpine` with volume for data persistence
3. Mosquitto service uses official `eclipse-mosquitto:2` with volume for configuration and data
4. MQTT2BDD service builds from production `Dockerfile` (not dev image)
5. All services use container names with `mqtt2bdd-prod-` prefix: `mqtt2bdd-prod-postgres`, `mqtt2bdd-prod-mosquitto`, `mqtt2bdd-prod-app`
6. MQTT2BDD service configured with environment variables via `.env.prod.example` file
7. Services connected via dedicated Docker network
8. Health checks configured for all services (PostgreSQL, Mosquitto, MQTT2BDD)
9. Restart policy set to `unless-stopped` for production resilience
10. `.env.prod.example` provided with all required environment variables documented
11. Manual test: Deploy full stack with `docker-compose -f docker-compose.prod.yml up`, verify all services start, publish MQTT message, verify persistence to PostgreSQL
12. Documentation in README includes production deployment instructions

### Story 3.3: Create Integration Test Infrastructure

**As a** QA engineer,
**I want** isolated integration test environment,
**so that** I can validate end-to-end functionality without polluting development data.

**Acceptance Criteria:**

1. `test/docker-compose.yml` defines isolated test stack: `test-postgres`, `test-mosquitto`, `test-mqtt2bdd`
2. All test containers use `mqtt2bdd-test-` prefix for clear separation from dev/prod
3. Test PostgreSQL uses ephemeral volume (no persistence) for clean state per test run
4. Test stack uses different ports to avoid conflicts with dev environment (e.g., PostgreSQL 5433 instead of 5432)
5. Test initialization script `test/init-db/01-schema.sql` creates same `sensor_metrics` schema
6. Test MQTT2BDD service builds from source with test configuration
7. `test/run-integration-tests.sh` script automates: start stack → wait for ready → publish test messages → verify DB → shutdown stack
8. Integration test verifies: MQTT subscription, message persistence, reconnection after simulated failures
9. Test script exits with code 0 on success, non-zero on failure (CI/CD compatible)
10. Manual test: Run `./test/run-integration-tests.sh`, verify all tests pass and containers clean up
11. All code passes `go fmt`, `go vet`, and `staticcheck`

### Story 3.4: Write Comprehensive Unit Tests

**As a** Go learner,
**I want** comprehensive unit tests demonstrating Go testing patterns,
**so that** I can learn idiomatic testing practices.

**Acceptance Criteria:**

1. Unit tests created for all core packages: `config`, `logger`, `mqtt`, `database`
2. `internal/config/config_test.go` uses table-driven tests for environment variable loading
3. `internal/mqtt/client_test.go` tests connection, subscription, and reconnection logic (using mocks or test broker)
4. `internal/database/client_test.go` tests connection pool, INSERT operations, error handling (using test database or mocks)
5. Tests demonstrate Go patterns: table-driven tests, test fixtures, subtests (`t.Run()`), test helpers
6. All tests pass with `go test ./...`
7. Tests include both success and failure scenarios (error paths)
8. Tests use `t.Parallel()` where appropriate for concurrent execution
9. Test coverage measured with `go test -cover ./...` (no specific percentage target, focus on quality)
10. Mock implementations documented for learning (e.g., how to mock MQTT client, database connections)
11. All code passes `go fmt`, `go vet`, and `staticcheck`

### Story 3.5: Write Educational README Documentation

**As a** Go learner,
**I want** comprehensive README documentation,
**so that** I can understand the architecture, deploy the application, and learn Go patterns from the codebase.

**Acceptance Criteria:**

1. `README.md` includes sections: Overview, Features, Architecture, Project Structure, Prerequisites, Quick Start, Configuration, Development, Production Deployment, Testing, Troubleshooting, Learning Resources
2. **Overview:** Concise description of MQTT2BDD purpose and value proposition
3. **Architecture:** Diagram (ASCII or embedded image) showing MQTT → Go App → PostgreSQL flow with goroutines and channels
4. **Project Structure:** Directory tree with explanations of each package's responsibility
5. **Prerequisites:** List required tools (Docker, Docker Compose, optionally Go 1.23+)
6. **Quick Start:** Step-by-step guide from clone to running application (dev environment)
7. **Configuration:** Table of all environment variables with descriptions, defaults, and examples
8. **Development:** Instructions for dev setup, debugging with Delve, running tests, code quality tools
9. **Production Deployment:** Instructions for deploying with production Docker Compose
10. **Testing:** How to run unit tests and integration tests
11. **Troubleshooting:** Common issues and solutions (connection failures, permission errors, etc.)
12. **Learning Resources:** Annotated links to Go documentation, MQTT specs, PostgreSQL guides
13. README includes badges: Go version, license (if applicable)
14. Code examples in README use syntax highlighting
15. Manual review: README is clear, complete, and actionable for a Go beginner

### Story 3.6: Add Inline Code Documentation for Learning

**As a** Go learner,
**I want** inline code comments explaining Go patterns and architectural decisions,
**so that** I can learn by reading the codebase.

**Acceptance Criteria:**

1. All exported functions, types, and methods have GoDoc comments following Go conventions
2. Complex goroutine interactions documented with comments explaining concurrency patterns
3. Channel operations annotated with buffering rationale and flow control explanations
4. Error handling patterns explained (e.g., "wrapping errors with context for debugging")
5. Key architectural decisions documented in package-level comments (e.g., `package mqtt` explains MQTT client responsibilities)
6. Code examples of Go idioms highlighted: defer for cleanup, table-driven tests, interface usage
7. `main.go` includes high-level application flow comments as a narrative guide
8. Non-obvious performance optimizations explained (e.g., connection pooling benefits)
9. Security considerations documented (e.g., why statically-linked binary, non-root user in Docker)
10. Generated GoDoc viewable with `go doc` or `godoc` tool
11. All code passes `go fmt`, `go vet`, and `staticcheck`

---

## Epic 4: Public Release & Binary Distribution

**Epic Goal:** Turn a working local application into a publicly installable one. Publish the
repository on GitHub under an MIT license, and automate the production of statically-linked
binaries for four platforms on every version tag, so that a home-automation user can download
and run MQTT2BDD with a single `curl` command — no clone, no Go toolchain, no Docker build.

This epic is brownfield: it was drafted after Epics 1-3 shipped. It is **additive in behaviour**
— the version-injection mechanism it depends on (`main.Version` overridden via `-ldflags`)
already exists from Story 3.1, and no application logic changes. It is **not** additive in files:
Story 4.1 rewrites the module path across every Go source file, because the maintainer's GitHub
account was renamed from `spydemon` to `hagnerk` after Epics 1-3 shipped, and a Go module path
must match the repository that serves it.

### Epic Context

**Existing system context:**

- Relevant functionality: the application already builds as a static binary
  (`CGO_ENABLED=0`, no libc dependency) and already reports an injectable version at startup
  (`main.Version`, defaulting to `dev`, overridden by `-ldflags "-X main.Version=…"`).
- Technology stack: Go 1.23, Docker Compose. No CI/CD exists — there is no `.github/` directory,
  no git remote, and no tag in the repository as of this epic's creation.
- Integration points: `.github/workflows/` (new), `LICENSE` (new), `README.md` (new sections),
  and the `go.mod` module path plus the import statement of every Go file, which must match the
  actual GitHub repository path `github.com/hagnerk/mqtt2bdd` for `go install` to resolve.

**Enhancement details:**

- What is added: an MIT license, a public GitHub repository, a tag-triggered GitHub Actions
  workflow that cross-compiles four targets and attaches the archives to a GitHub Release, and
  the installation documentation that makes those archives usable.
- How it integrates: entirely through new files. The workflow reuses the exact build flags of
  the production `Dockerfile` (`CGO_ENABLED=0`, `-s -w -X main.Version=…`), so a released binary
  and a container-built binary are produced identically.
- Success criteria: from a clean machine with no Go and no Docker, `curl`-ing the release URL,
  extracting the archive and running the binary yields a process that logs
  `event=starting version=1.0.0` and connects to a configured broker and database.

**Out of scope (deliberate):** publishing the Docker image to a registry (GHCR or Docker Hub).
`docker-compose.prod.yml` keeps building the image locally with `--build`. This was considered
and set aside to keep the epic focused on the downloadable-binary use case; it remains a
candidate for a later epic.

**Compatibility requirements:**

- The only change to files under `cmd/` and `internal/` is the module path in their `import`
  blocks (Story 4.1). No logic, signature, or behaviour changes; the test suite must pass
  unchanged before and after.
- The three Docker Compose stacks (`dev/`, `test/`, `docker-compose.prod.yml`) keep working
  exactly as documented; the release workflow is an additional path to a binary, not a
  replacement for any existing one.
- `.gitignore` keeps ignoring `.env`, `.env.prod`, `*.env`, `bin/` and `__debug_bin*`, so no
  build artefact or credential can reach the public repository.

**Risk mitigation:**

- **Primary risk:** making the repository public is irreversible with respect to what has
  already been pushed — a secret in the git history stays retrievable even after deletion.
  *Mitigation:* Story 4.2 gates the push behind an explicit audit of the full history. That
  audit has already been run during epic drafting and came back clean: the only tracked
  environment files are `dev/.env.example` and `.env.prod.example` (templates with no real
  values), and the only credential-shaped strings in the tree are test fixtures. The commit
  author email `kevin.hagner@spyzone.fr` will become public — a professional address, accepted.
- **Secondary risk:** a broken workflow publishes a release with missing or non-functional
  assets, and a GitHub Release tag is awkward to retract once people have fetched it.
  *Mitigation:* Story 4.3 requires an end-to-end rehearsal on a pre-release tag (`v0.9.0-rc1`)
  before `v1.0.0` is ever tagged, and gates the build behind the project's existing quality
  checks so a failing test never produces a release.
- **Rollback plan:** each story is independently revertible. Deleting `.github/workflows/`
  removes the automation with no trace in the application; deleting a tag and its GitHub Release
  removes a published version; the repository can be switched back to private. Only the
  publication of already-pushed history is irreversible, which is why 4.1 audits before pushing.

### Story 4.1: Rename the Go Module Path to the New GitHub Account

**As a** maintainer whose GitHub account was renamed from `spydemon` to `hagnerk`,
**I want** the module path and every reference to it updated across the repository,
**so that** the module resolves to the repository that actually serves it.

**Context:** a Go module path is not decorative — it is the URL the toolchain fetches from.
Leaving it as `github.com/spydemon/mqtt2bdd` and relying on GitHub's account-rename redirect is
not an option: a freed username can be re-registered by anyone, and the module path would then
resolve to a stranger's repository. Doing this now is free — the module has never been published,
has no tag and no remote, so there is no downstream consumer, no `retract` directive and no `/v2`
path semantics to handle. That window closes the moment Story 4.2 pushes.

**Acceptance Criteria:**

1. `go.mod`'s module declaration reads `module github.com/hagnerk/mqtt2bdd`
2. Every `import` referencing the old path is updated across the 9 affected Go files
   (`cmd/mqtt2bdd/main.go`, `cmd/mqtt2bdd/integration_helpers_test.go`,
   `internal/database/client.go`, `internal/database/client_test.go`,
   `internal/database/integration_test.go`, `internal/logger/logger_test.go`,
   `internal/config/config_test.go`, `internal/mqtt/client.go`, `internal/mqtt/client_test.go`)
3. The rename is propagated to every remaining occurrence in the repository: `README.md`, the 19
   story files under `docs/stories/`, and the 4 gate files under `docs/qa/gates/`. Completed
   stories and gates are included deliberately — a module path is a factual identifier, not a
   dated opinion, and a reader copying `go mod init github.com/spydemon/mqtt2bdd` out of Story
   1.2 would be actively misled
4. `grep -ri spydemon .` returns matches only where the old name is named *as* the old name —
   Epic 4 of `docs/prd.md` and this story's own record, which document why the rename happened.
   No occurrence remains as a live path anywhere else. Git history is out of scope: past commits
   are immutable and are not rewritten
5. Verification passes through the containerized toolchain, from `dev/`, per this project's
   workflow — never on the host: `docker compose exec go-dev go build ./...`,
   `docker compose exec go-dev go test ./...`, `docker compose exec go-dev go vet ./...`,
   `docker compose exec go-dev gofmt -l .`
6. The integration suite passes unchanged: `./test/run-integration-tests.sh` from the repository
   root
7. The change is mechanical and isolated: the commit contains the path rewrite and nothing else —
   no logic change, no signature change, no opportunistic cleanup — so it can be reviewed at a
   glance and reverted with a single `git revert`

### Story 4.2: Prepare the Repository for Public Release

**As a** maintainer,
**I want** the repository licensed, audited, and published on GitHub,
**so that** anyone can read the code and a release workflow has somewhere to publish to.

**Acceptance Criteria:**

1. `LICENSE` exists at the repository root, containing the unmodified MIT license text with
   copyright line `Copyright (c) 2026 Kevin Hagner`
2. `README.md` carries an MIT license badge next to the existing Go version badge, linking to
   the `LICENSE` file — this closes Story 3.5 AC13, which left the license badge conditional
   ("if applicable") because no license existed at the time
3. `README.md` gains a short **License** section at the end of the document, stating the license
   and pointing to the `LICENSE` file
4. Full git history audited for secrets before the first push: no real credential, private key,
   or non-template environment file is present in any commit. The audit is recorded in the
   story's Dev Agent Record, including the command used, so it is reproducible
5. `.gitignore` verified to still cover `.env`, `.env.prod`, `*.env`, `bin/` and `__debug_bin*`,
   and `git status` is clean before the push
6. A public GitHub repository exists at `github.com/hagnerk/mqtt2bdd`, matching exactly the
   `go.mod` module declaration set in Story 4.1 — any divergence and
   `go install github.com/hagnerk/mqtt2bdd/...` cannot resolve. Story 4.1 must be merged first;
   pushing an unrenamed module publishes a path that cannot be fixed retroactively for anyone
   who has already fetched it
7. Remote `origin` points to that repository and the full `main` branch history is pushed
8. The GitHub repository has a one-line description and topics set (`go`, `mqtt`, `postgresql`,
   `home-automation`, `iot`, `docker`) for discoverability
9. `README.md`'s Quick Start replaces the `<repository-url>` placeholder in the `git clone`
   command with the real repository URL
10. The pushed repository renders correctly on GitHub: README diagrams and tables display as
    intended, and every relative link in the documentation resolves

### Story 4.3: Automate Cross-Platform Binary Builds on Tag

**As a** maintainer,
**I want** a tagged version to automatically produce downloadable binaries for four platforms,
**so that** publishing a release is one `git push --tags` and never a manual build.

**Acceptance Criteria:**

1. `.github/workflows/release.yml` exists, triggered only by pushed tags matching `v*.*.*`
   (including pre-release suffixes such as `v0.9.0-rc1`)
2. The workflow runs the project's existing quality gates in a first job that the build and
   publish jobs depend on, so no artefact is built and no release is created if any gate fails:
   - static analysis and unit tests: `gofmt -l .` (must report nothing), `go vet ./...`,
     `staticcheck ./...`, `go test ./...`
   - the integration suite: `./test/run-integration-tests.sh`, run from the repository root as
     documented. The script is already self-contained and CI-friendly from Story 3.3 — it brings
     the isolated `test/` stack up, waits for health, runs the `//go:build integration` suite,
     tears everything down unconditionally, and exits non-zero on any failure — so it needs no
     workflow-specific adaptation beyond a runner that provides Docker Compose
   These gates run on GitHub's runners, not on a contributor's machine: this is a publication
   gate, not a git hook. A failing gate leaves the pushed tag and commits in place and blocks
   only the release; recovering means fixing, deleting the tag locally and remotely, and
   re-tagging. The AC12 pre-release rehearsal exists so that this is never first discovered on
   `v1.0.0`
3. A build matrix produces one static binary per target: `linux/amd64`, `linux/arm64`,
   `linux/arm` (with `GOARM=7`, for 32-bit Raspberry Pi OS), and `darwin/arm64`
4. Every build uses `CGO_ENABLED=0` and `-ldflags="-s -w -X main.Version=<version>"`, where
   `<version>` is the tag with its leading `v` stripped (`v1.0.0` → `1.0.0`) — identical flags to
   the production `Dockerfile`, so a released binary matches a container-built one
5. Each target is packaged as a `.tar.gz` archive under a **canonical, version-bearing name**:
   `mqtt2bdd_<version>_linux_amd64.tar.gz`, `mqtt2bdd_<version>_linux_arm64.tar.gz`,
   `mqtt2bdd_<version>_linux_armv7.tar.gz`, `mqtt2bdd_<version>_darwin_arm64.tar.gz` — so that a
   downloaded file states on disk which version it holds
6. Each archive is **also** published under a version-free alias name
   (`mqtt2bdd_linux_amd64.tar.gz`, `mqtt2bdd_linux_arm64.tar.gz`, `mqtt2bdd_linux_armv7.tar.gz`,
   `mqtt2bdd_darwin_arm64.tar.gz`), byte-identical to its canonical counterpart. The alias exists
   solely to make the `releases/latest/download/<name>` URL resolvable — that URL requires an
   asset name that does not change between releases. A release therefore carries eight archives,
   four distinct payloads
7. Each archive contains the `mqtt2bdd` binary (executable bit set), `LICENSE`, and `README.md`
8. A `checksums.txt` asset lists the SHA-256 of all eight archives in the standard
   `sha256sum -c` input format. Each canonical/alias pair necessarily shares one checksum, since
   the payloads are identical; the file lists both names so that either download can be verified
   without knowing about the other
9. The workflow creates a GitHub Release on the tag, attaches the eight archives plus
   `checksums.txt`, and uses GitHub's auto-generated release notes as the body
10. A tag carrying a pre-release suffix (`-rc`, `-beta`) produces a release marked as
    pre-release, so it never becomes the target of the `latest` URL — the aliases it publishes
    are reachable only through its own pinned tag URL, which is the intent
11. The workflow declares `permissions: contents: write` and authenticates with the default
    `GITHUB_TOKEN` only — no additional secret is configured in the repository
12. The workflow is rehearsed end to end on a `v0.9.0-rc1` tag before `v1.0.0` is tagged: the
    run succeeds, all nine assets are present, a canonical and an alias archive verify to the
    same SHA-256, and the extracted `linux/amd64` binary runs and logs
    `event=starting version=0.9.0-rc1`
13. Workflow YAML is commented to the standard of the rest of the repository — each job and each
    non-obvious step (the `v`-stripping, `GOARM=7`, why each archive is published under two
    names) explains its rationale, consistent with this project's educational intent

### Story 4.4: Document Binary Installation from a GitHub Release

**As a** home-automation user with no Go toolchain,
**I want** copy-pasteable installation instructions,
**so that** I can download, verify, and run MQTT2BDD without building anything.

**Acceptance Criteria:**

1. `README.md` gains an **Installation** section, placed before Quick Start, presented as the
   path for running the application and Quick Start as the path for developing on it
2. The section documents both download forms and states plainly when to use which:
   - **Pinned** (recommended for anything scripted or deployed):
     `.../releases/download/v1.2.3/mqtt2bdd_1.2.3_linux_amd64.tar.gz`. The tag sits in the URL,
     so this URL keeps serving 1.2.3 forever — a later 2.0.0 with breaking configuration changes
     can never be substituted underneath a script that pinned
   - **Latest** (for a first manual try):
     `.../releases/latest/download/mqtt2bdd_linux_amd64.tar.gz`, which follows the newest
     non-pre-release version automatically, with the trade-off that it can change under you
3. The section states that GitHub resolves no semver ranges server-side — there is no `^1.3`
   equivalent — and shows the client-side pattern for a script that wants one: list releases via
   `api.github.com/repos/hagnerk/mqtt2bdd/releases`, select the tag in the wanted range, then
   build the pinned URL from it
4. A table maps each of the four platforms to its canonical and alias archive names and to the
   hardware it targets, stating explicitly that `linux_armv7` is for 32-bit Raspberry Pi OS and
   `linux_arm64` for 64-bit
5. Checksum verification is documented as a runnable sequence using `checksums.txt` and
   `sha256sum --ignore-missing -c checksums.txt` — `--ignore-missing` is required and explained,
   since `checksums.txt` lists all eight archives while a user has downloaded only one
6. The section states that the Linux binaries are statically linked and therefore need no
   runtime dependency, and that the macOS binary is unsigned — first run requires clearing the
   Gatekeeper quarantine attribute, with the command given
7. The section states what the binary still needs to run: a reachable MQTT broker, a PostgreSQL
   database with the `sensor_metrics` schema applied, and the environment variables of the
   Configuration section, to which it links. A binary alone is not a working system
8. `README.md`'s Project Structure tree includes the new `.github/workflows/` directory and the
   `LICENSE` file, with one-line descriptions
9. `README.md`'s Development section documents how to cut a release: tag with `vX.Y.Z`, push the
   tag, and let the workflow publish — including the pre-release rehearsal convention
10. Every URL and command in the new sections is verified against the actual published release,
    not written from assumption

---

## Epic 5: Write Pipeline Hardening

**Epic Goal:** Guarantee that no single MQTT message can stop MQTT2BDD from persisting the others. Today one payload PostgreSQL refuses is retried forever by the only database writer, the buffer fills, reception blocks, and — because the offending message is retained by the broker — a restart reproduces the stall at once.

This epic is brownfield and corrective: it was drafted after v1.0.0 shipped, following a production incident on `zigbee2mqtt/bridge/definitions` (SQLSTATE 22P05, `\u0000` in a 283 KB payload). The full analysis, including a probe of what PostgreSQL 15 actually rejects, is recorded in `docs/sprint-change-proposals/2026-09-17-write-pipeline-stall.md`.

### Epic Context

**Existing system context:**

- `insertWithRetry` (`cmd/mqtt2bdd/main.go`) retries every insert error every 10 seconds without limit, and `dbWriterLoop` is the single consumer of the buffer.
- `InsertMessage` (`internal/database/queries.go`) forwards the payload to a `JSONB NOT NULL` column as `json.RawMessage`; `pgx` does not validate it, so PostgreSQL is the only judge of its content.
- `docs/architecture/error-handling-strategy.md` already defines a *Data Integrity (Log and Skip)* category (§3) that was never implemented: this epic implements it.
- The subscription filter is hard-coded to `#`; no configuration exists to narrow it.

**Enhancement details:**

- Story 5.1 separates permanent from transient write errors. Story 5.2 repairs the content that can be repaired. Story 5.3 lets operators exclude topics.
- Release plan: 5.1 and 5.2 ship as **v1.0.1** (bug fixes); 5.3 ships as **v1.1.0** (new configuration variable).

**Compatibility requirements:**

- No schema change. No new Go dependency.
- A valid payload is written exactly as today: same SQL, same retry behaviour on outages, same log entries.
- `MQTT_EXCLUDE_TOPICS` is optional; when unset, the application subscribes to and stores everything, as in v1.0.0.

**Risk mitigation:**

- **Primary risk:** classifying a transient error as permanent silently drops messages that would have been stored after recovery. *Mitigation:* the permanent set is a closed list of three SQLSTATE classes, all describing the value being written. Classes that describe the environment stay retried — connection (08), resources (53), operator intervention (57), system (58), transaction conflicts (40), internal (XX) — and so do class 42 errors (missing privilege, missing table). A class 42 error hits every message identically, and an operator can fix it, as happened on 2026-09-17 with a missing grant.
- **Secondary risk:** stored JSON differs from what the device published. *Mitigation:* only content PostgreSQL would refuse is touched, always with the visible U+FFFD, and every repair is logged. `jsonb` never stored payloads byte-for-byte anyway: it reorders keys, drops duplicate keys and normalises whitespace.
- **Rollback plan:** each story is independently revertible with `git revert`. Reverting 5.1 restores the stall.

### Story 5.1: Stop Retrying Writes the Database Can Never Accept

**As an** operator,
**I want** a message the database can never store to be discarded and logged instead of retried forever,
**so that** one bad payload cannot stop every other message from being persisted.

**Acceptance Criteria:**

1. Write errors are classified inside `internal/database` as either *permanent* or *transient*. The classification is exposed to callers as a sentinel error matchable with `errors.Is` (for example `database.ErrRejected`), so `cmd/mqtt2bdd` never imports `pgconn` or reasons about SQLSTATE codes
2. An error is permanent if and only if it is a `*pgconn.PgError` (found with `errors.As`, so wrapping does not hide it) whose SQLSTATE class is `22`, `23` or `54`. Every other error is transient, including non-PostgreSQL errors, context deadlines, and SQLSTATE classes `08`, `40`, `42`, `53`, `57`, `58` and `XX`
3. Before any database round-trip, a payload that fails `json.Valid` is rejected locally with the same sentinel, without contacting the database
4. A zero-length payload (MQTT's way of clearing a retained message) is skipped before any round-trip: logged at DEBUG with `event=write_skipped` and `reason=empty_payload`, not counted as rejected, not treated as an error by the caller
5. When the error is permanent, `insertWithRetry` returns immediately — no 10-second wait — on both the steady-state path and the shutdown drain path, and the writer moves on to the next buffered message
6. A rejection is logged once, at ERROR, with `event=write_rejected`, `operation=insert`, `topic`, `payload_size`, `duration_us`, `reason` (`invalid_json` for a local rejection, `sqlstate` for a server one), `sqlstate` (server rejections only) and `error`. The payload body is never logged. `write_failure` keeps meaning "this write will be retried" and is no longer emitted for rejections
7. A server-side rejection proves the database answered, so it leaves the client connected: `connected` is set to `true`, never `false`. `lastWriteAt` and `writeCount` are not updated, because nothing was written
8. The database client exposes a monotonic rejection counter, and the `health_check` entry gains a `rejected_last_interval` field computed like `processed_last_interval`
9. Unit tests cover the classification with synthetic `*pgconn.PgError` values — at least `22P05`, `22P02`, `22021`, `22003`, `23502` and `54001` as permanent, and `08006`, `40P01`, `42501`, `42P01`, `53100`, `57P01` and `XX000` as transient — plus a wrapped `PgError`, a plain error and `context.DeadlineExceeded`
10. An integration test publishes, in this order, a non-JSON payload, a payload PostgreSQL rejects server-side (`{"v":1e1000000}`, which passes `json.Valid` and keeps failing after Story 5.2), an empty payload, and a valid payload. The valid row is stored well within the 10-second retry interval, the first three are absent, and two `write_rejected` entries are logged
11. `docs/architecture/error-handling-strategy.md`, `docs/architecture/logging-standards.md` and the README's troubleshooting and data-loss sections describe the new behaviour
12. Verification passes through the containerized toolchain from `dev/` — `gofmt -l .`, `go vet ./...`, `staticcheck ./...`, `go test ./...` — and `./test/run-integration-tests.sh` passes from the repository root

### Story 5.2: Repair Payload Content PostgreSQL Refuses

**As an** operator,
**I want** payload content that PostgreSQL refuses but that can be repaired safely to be repaired rather than discarded,
**so that** a large, legitimate message such as `zigbee2mqtt/bridge/definitions` is stored instead of lost.

**Acceptance Criteria:**

1. `internal/database` repairs the payload before the checks of Story 5.1, replacing each defect with U+FFFD:
   - a JSON `\u0000` escape becomes `\uFFFD`, in keys and values alike, and only when its backslash starts an escape, meaning it is preceded by an even number of backslashes (zero included). `"\\u0000"` is literal text and stays untouched, while `"\\\u0000"` is repaired
   - an unpaired UTF-16 surrogate escape becomes `\uFFFD`: a high surrogate (`\uD800`–`\uDBFF`) not immediately followed by a low surrogate escape, or a low surrogate (`\uDC00`–`\uDFFF`) not immediately preceded by a high one. Valid pairs stay untouched, and hex digits are matched case-insensitively
   - each maximal invalid UTF-8 byte sequence (including CESU-8 encoded surrogates and overlong forms) becomes the UTF-8 encoding of U+FFFD
2. Nothing else is transformed. Escaped control characters `\u0001`–`\u001F`, noncharacters such as `\uFFFF`, and valid surrogate pairs are accepted by PostgreSQL and pass through byte-for-byte. Numeric overflow and structural errors are not repairable and remain rejected by Story 5.1
3. A payload needing no repair is returned unchanged without allocating, verified by a test using `testing.AllocsPerRun`
4. A repaired message is logged once at WARN with `event=payload_sanitized`, no `operation` (it reports a state, not a failed attempt), `topic`, `payload_size`, and one count per repair kind: `nul_escapes`, `lone_surrogates`, `invalid_utf8_sequences`
5. Unit tests cover every repairable case of the proposal's probe table, backslash runs of length 1 to 4 before `u0000`, an escape at the very end of the payload, several defects in one payload, and a payload containing none
6. An integration test publishes `{"a":"x\u0000y"}`, a lone surrogate and a raw `0xff` byte, and finds each stored with U+FFFD in place of the defect
7. The README states that stored JSON may differ from the published payload in this way, and that the database must use the `UTF8` encoding
8. Verification passes as in Story 5.1, AC12

### Story 5.3: Exclude Topics from Persistence

**As an** operator,
**I want** to list MQTT topic filters whose messages are not stored,
**so that** configuration traffic such as `zigbee2mqtt/bridge/#` does not fill `sensor_metrics`.

**Acceptance Criteria:**

1. A new optional environment variable `MQTT_EXCLUDE_TOPICS` holds a comma-separated list of MQTT topic filters. Surrounding whitespace is trimmed, empty entries are ignored, and an unset or empty value excludes nothing
2. Each filter is validated at startup against MQTT 3.1.1 §4.7: `#` only as a whole, final level; `+` only as a whole level; no NUL character. An invalid filter aborts startup with `event=config_load_failed`, naming the offending filter
3. Matching follows MQTT semantics: `+` matches exactly one level; `#` matches the parent level and any number of child levels (`a/#` matches `a`); empty levels are significant; matching is case-sensitive; a topic starting with `$` is not matched by a filter starting with a wildcard
4. An excluded message is dropped in the reception handler before it reaches the buffer, and is not counted by `processed_last_interval`
5. Each exclusion is logged at DEBUG with `event=message_excluded`, `topic` and the matching `filter`; nothing is logged per message at INFO
6. The `config_loaded` entry reports the parsed list as `exclude_topics`
7. The subscription stays `#`: MQTT has no negative subscription, so excluded payloads still cross the network. The README states this
8. `MQTT_EXCLUDE_TOPICS` is documented in the README configuration table (with `zigbee2mqtt/bridge/#` as the example), in `.env.prod.example`, and passed through in `docker-compose.prod.yml`. `docs/architecture/components.md` and `docs/architecture/logging-standards.md` are updated
9. Unit tests cover filter validation, the matching rules above including the examples of MQTT 3.1.1 §4.7, and configuration parsing. An integration test shows that a message on an excluded topic is absent and a message on another topic is stored
10. Verification passes as in Story 5.1, AC12

---
