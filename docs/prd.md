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

| Date | Version | Description | Author |
|------|---------|-------------|--------|
| 2026-01-23 | 0.1.0 | Initial PRD creation from Project Brief | PM Agent (John) |

---

## Requirements

### Functional Requirements

1. **FR1:** The application must subscribe to all MQTT topics using wildcard (`#`) on a configured MQTT broker and receive messages in real-time
2. **FR2:** The application must parse each MQTT message to extract topic (sensor identifier), timestamp, and JSON payload
3. **FR3:** The application must insert normalized records into PostgreSQL `sensor_metrics` table with columns: sensor (VARCHAR), date (TIMESTAMP), metrics (JSONB)
4. **FR4:** The application must detect MQTT broker disconnections and automatically reconnect with fixed 10-second retry interval
5. **FR5:** The application must detect PostgreSQL database disconnections and automatically reconnect with fixed 10-second retry interval
6. **FR6:** The application must buffer incoming MQTT messages in memory (capacity: 1000 messages) to handle temporary database outages without message loss
7. **FR7:** The application must handle SIGTERM/SIGINT signals gracefully, flushing all buffered messages to database before shutdown
8. **FR8:** The application must accept configuration exclusively via environment variables (MQTT host/port/credentials, PostgreSQL connection string, log level)
9. **FR9:** The application must implement structured logging with levels (INFO, DEBUG, ERROR) for startup, shutdown, connection events, message processing, and errors
10. **FR10:** The application must log all critical lifecycle events including: application start/stop, MQTT connection established/lost, database connection established/lost, and write failures with context

### Non-Functional Requirements

1. **NFR1:** The application must run reliably with automatic recovery from MQTT broker and database connection failures
2. **NFR2:** The application must be deployable as a Docker container based on Alpine Linux 3.23 with a statically-linked Go binary
3. **NFR3:** The application must have comprehensive unit tests demonstrating Go testing best practices for core components (MQTT handler, database writer, reconnection logic)
4. **NFR4:** All code must pass `go fmt` and `go vet` checks, following Go effective conventions and idiomatic patterns
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
  - PostgreSQL container (`mqtt2bdd-postgres`) for database
  - Mosquitto container (`mqtt2bdd-mosquitto`) for MQTT broker
  - Go development container (`mqtt2bdd-go-dev`) with volume-mounted project directory for live code editing
  - **Container naming:** All containers prefixed with `mqtt2bdd-` for easy identification among other projects
  - **Delve debugger integration:** Go container configured with Delve (dlv) for remote debugging via exposed port (equivalent to XDebug workflow familiar to PHP developers)
  - One-command startup: `docker-compose up` in `dev/` directory
  - Hot reload capability for rapid development iteration
- **Database Schema:** Pre-existing `sensor_metrics` table with schema: `sensor VARCHAR, date TIMESTAMP, metrics JSONB` (application does NOT create schema, assumes it exists)
- **Deployment Model:** Single Docker container, configured entirely via environment variables, suitable for Docker Compose or Kubernetes deployment
- **Concurrency Model:** Goroutines + channels for concurrent MQTT message handling and database writes, demonstrating Go's CSP (Communicating Sequential Processes) paradigm
- **Error Handling Philosophy:** Explicit error returns following Go conventions, no exceptions/panics in normal operation, all errors logged with context
- **Dependency Management:** Go modules (`go.mod`) with minimal external dependencies (only Paho MQTT client and pgx driver)

---

## Epic List

### Epic 1: Containerized Dev Environment & Core MQTT-to-PostgreSQL Pipeline

Establish fully containerized development environment with three containers (PostgreSQL, Mosquitto, Go dev environment with Delve debugging support), initialize Go project structure following best practices, and deliver a working MQTT subscription handler that captures messages and writes them to PostgreSQL.

### Epic 2: Production Resilience & Reliability

Implement automatic reconnection logic for MQTT and database failures, message buffering, graceful shutdown handling, and comprehensive structured logging to create an autonomous, self-healing service.

### Epic 3: Production Docker Image & Educational Documentation

Create optimized production Dockerfile (multi-stage Alpine build distinct from dev container), establish integration testing infrastructure with isolated test docker-compose stack, and write comprehensive educational documentation covering architecture, deployment, debugging setup, and Go patterns walkthrough.

---

## Epic 1: Containerized Dev Environment & Core MQTT-to-PostgreSQL Pipeline

**Epic Goal:** Establish a fully functional development environment where developers can start contributing immediately with one command, and deliver the core value proposition—MQTT messages flowing into PostgreSQL—demonstrating the fundamental data pipeline that solves the home automation persistence problem.

### Story 1.1: Setup Containerized Development Environment

**As a** developer learning Go,
**I want** a one-command development environment setup with PostgreSQL, Mosquitto, and Go containers,
**so that** I can start coding immediately without installing any infrastructure locally.

**Acceptance Criteria:**

1. `dev/docker-compose.yml` defines three services: `postgres`, `mosquitto`, and `go-dev`
2. All containers use `container_name` directive with `mqtt2bdd-` prefix: `mqtt2bdd-postgres`, `mqtt2bdd-mosquitto`, `mqtt2bdd-go-dev` for easy identification among other projects
3. PostgreSQL container uses official `postgres:15-alpine` image with environment variables for database name, user, and password
4. Mosquitto container uses official `eclipse-mosquitto:2` image with basic configuration allowing anonymous connections
5. Go development container based on `golang:1.23-alpine` (latest stable) with Delve debugger installed
6. Go container exposes port 2345 for Delve remote debugging
7. Go container mounts project root directory as volume (e.g., `.:/app`) for live code editing
8. SQL initialization script `dev/init-db/01-schema.sql` creates `sensor_metrics` table with schema: `sensor VARCHAR(255), date TIMESTAMP, metrics JSONB`
9. PostgreSQL container automatically executes initialization scripts on first startup
10. All containers start successfully with `docker-compose up` from `dev/` directory
11. Developer can verify PostgreSQL connectivity: `docker exec -it mqtt2bdd-postgres psql -U <user> -d <dbname> -c "\dt"` shows `sensor_metrics` table
12. Developer can verify Mosquitto connectivity: `docker exec -it mqtt2bdd-mosquitto mosquitto_sub -t '#' -v` listens to all topics
13. `.gitignore` includes `.env` file and Docker-related temporary files
14. `dev/.env.example` provides template for required environment variables with documentation

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
7. All code passes `go fmt` and `go vet` with zero warnings
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
9. All code passes `go fmt` and `go vet`

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
9. `main.go` logs application startup at INFO level with version information (hardcoded "v0.1.0" for now)
10. All code passes `go fmt` and `go vet`

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
9. Manual test: Run application with `docker-compose up`, verify logs show successful MQTT connection to `mqtt2bdd-mosquitto` container
10. Application exits gracefully if MQTT connection fails with clear error message
11. All code passes `go fmt` and `go vet`

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
7. Manual test: Use `mosquitto_pub` from mqtt2bdd-mosquitto container to publish test message: `docker exec mqtt2bdd-mosquitto mosquitto_pub -t 'test/topic' -m '{"temp": 20.5}'`
8. Verify application logs show received message with correct topic and payload preview
9. Message reception runs in separate goroutine (non-blocking)
10. All code passes `go fmt` and `go vet`

### Story 1.7: Implement PostgreSQL Client Connection

**As a** developer,
**I want** to establish a connection to PostgreSQL database with connection pooling,
**so that** I can persist MQTT messages efficiently.

**Acceptance Criteria:**

1. `internal/database/client.go` defines `Client` struct wrapping pgx connection pool
2. `go.mod` includes dependency: `github.com/jackc/pgx/v5` and `github.com/jackc/pgx/v5/pgxpool`
3. `NewClient(config)` creates connection pool using DSN constructed from config: `postgres://user:pass@host:port/dbname`
4. `Connect(ctx context.Context)` method establishes connection pool with context support
5. Connection pool configuration: min connections = 2, max connections = 10
6. `Connect()` logs connection attempt (INFO) and success/failure (ERROR) with database host and name
7. `Ping(ctx context.Context)` method verifies database connectivity
8. `Close()` method cleanly closes connection pool
9. `main.go` updated to create database client, connect on startup, defer close on exit
10. Manual test: Run application, verify logs show successful PostgreSQL connection to `mqtt2bdd-postgres` container
11. Application exits gracefully if database connection fails with clear error message including connection details
12. All code passes `go fmt` and `go vet`

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
10. All code passes `go fmt` and `go vet`

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
6. Manual end-to-end test: Publish MQTT message from mqtt2bdd-mosquitto container: `docker exec mqtt2bdd-mosquitto mosquitto_pub -t 'zigbee2mqtt/living_room/thermostat' -m '{"temperature": 21.3, "humidity": 45}'`
7. Verify message appears in PostgreSQL sensor_metrics table with correct topic, timestamp, and JSON payload
8. Manual load test: Publish 100 messages rapidly, verify all messages persisted correctly
9. Application startup logs show clear sequence: Config loaded → Logger initialized → MQTT connected → PostgreSQL connected → Subscribed to topics
10. All code passes `go fmt` and `go vet`

