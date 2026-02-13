# MQTT2BDD Architecture Document

## Introduction

This document outlines the overall project architecture for MQTT2BDD, including backend systems, shared services, and non-UI specific concerns. Its primary goal is to serve as the guiding architectural blueprint for AI-driven development, ensuring consistency and adherence to chosen patterns and technologies.

**Relationship to Frontend Architecture:**
MQTT2BDD is a backend-only service with no user interface components. This document serves as the complete architectural specification.

### Starter Template or Existing Project

**Decision:** N/A - No starter template required for this Go project.

**Rationale:**
- Go projects don't typically use heavy scaffolding tools like Create React App or Vue CLI
- The PRD already specifies the exact directory structure (cmd/, internal/, pkg/)
- Starting from scratch provides better learning opportunities (one of the stated project goals)
- The dev environment is fully containerized, so no local Go installation complexity

### Change Log

| Date | Version | Description | Author |
|------|---------|-------------|--------|
| 2026-02-13 | 1.0 | Initial architecture document creation | Winston (Architect Agent) |

---

## High Level Architecture

### Technical Summary

MQTT2BDD implements a **resilient event-driven pipeline** architecture using Go's native concurrency primitives. The system acts as a stateless bridge between MQTT and PostgreSQL, employing goroutines and channels to achieve concurrent message ingestion and persistent storage. Core technology choices include Go 1.23+ for CSP-based concurrency, Eclipse Paho for MQTT protocol handling, and pgx/v5 for high-performance PostgreSQL integration. The architecture emphasizes automatic failure recovery, graceful degradation through buffering, and operational observability via structured logging—directly supporting the PRD goals of autonomous operation and educational codebase design.

### High Level Overview

**1. Architectural Style:** Event-Driven Monolith with Goroutine-Based Concurrency

**2. Repository Structure:** Monorepo (as specified in PRD Technical Assumptions)
- Single repository containing all application code, configuration, and deployment artifacts
- Standard Go project layout with `cmd/`, `internal/`, `pkg/` separation

**3. Service Architecture:** Monolith (as specified in PRD Technical Assumptions)
- Single-process Go application with three concurrent goroutines:
  - **Main goroutine:** Application lifecycle, signal handling, coordination
  - **MQTT receiver goroutine:** Subscribe to wildcard topics, publish to internal channel
  - **Database writer goroutine:** Consume from channel, persist to PostgreSQL
- No microservices, no inter-process communication

**4. Primary Data Flow:**
```
MQTT Broker (Zigbee2MQTT)
  → MQTT Client Goroutine (subscribe #)
  → Buffered Channel (1000 capacity)
  → Database Writer Goroutine
  → PostgreSQL (sensor_metrics table)
```

**5. Key Architectural Decisions:**

- **Goroutine + Channel Pipeline:** Decouples message reception from database writes, enabling buffering during DB outages and maximizing throughput
- **Wildcard MQTT Subscription (`#`):** Captures all topics without hardcoding device names, supporting dynamic device addition
- **Pre-existing Schema Assumption:** Application does not manage schema migrations, assumes `sensor_metrics` table exists (operational simplicity)
- **Buffered Channel (1000 messages):** Provides ~5-10 minutes of buffer time during database outages (assuming typical message rate)
- **Fixed Retry Intervals (10 seconds):** Simple, predictable reconnection behavior without exponential backoff complexity
- **Environment-Only Configuration:** Zero-dependency configuration using stdlib, no config files to manage

### High Level Project Diagram

```mermaid
graph TB
    subgraph "External Systems"
        MQTT[MQTT Broker<br/>Mosquitto<br/>Port 1883]
        ZigBee[Zigbee2MQTT Devices<br/>~100 sensors/actuators]
        Grafana[Grafana<br/>Visualization]
    end

    subgraph "MQTT2BDD Application"
        Main[Main Goroutine<br/>Lifecycle & Signals]
        MQTTClient[MQTT Client Goroutine<br/>Paho Client]
        Channel[Buffered Channel<br/>1000 messages]
        DBWriter[DB Writer Goroutine<br/>pgx Pool]
    end

    subgraph "Persistence"
        PG[(PostgreSQL<br/>sensor_metrics table)]
    end

    ZigBee -->|publish| MQTT
    MQTT -->|subscribe '#'| MQTTClient
    MQTTClient -->|send Message| Channel
    Channel -->|receive Message| DBWriter
    DBWriter -->|INSERT| PG
    PG -.->|queries| Grafana
    Main -.->|coordinates| MQTTClient
    Main -.->|coordinates| DBWriter

    style Main fill:#e1f5ff
    style MQTTClient fill:#fff4e1
    style DBWriter fill:#fff4e1
    style Channel fill:#ffe1e1
```

### Architectural and Design Patterns

Based on the PRD requirements and Go best practices, here are the key patterns:

- **Communicating Sequential Processes (CSP):** Go's native goroutine + channel model for concurrent message processing - _Rationale:_ Idiomatic Go concurrency, provides natural backpressure through buffered channels, educational value for learning Go concurrency

- **Pipeline Pattern:** MQTT → Channel → Database as staged data flow - _Rationale:_ Decouples producers from consumers, enables buffering, simplifies error handling in each stage independently

- **Fail-Fast with Retry:** Explicit error returns with automatic reconnection loops - _Rationale:_ Aligns with Go error handling conventions, provides resilience without hiding failures, clear operational visibility

- **Repository Pattern (Lightweight):** Database client abstracts pgx connection pool and SQL operations - _Rationale:_ Enables testing with mocks, centralizes database logic, follows Go best practices for package organization

- **Configuration via Environment:** Twelve-Factor App principle using `os.Getenv()` - _Rationale:_ Zero dependencies, Docker-native configuration, simple deployment across environments

- **Structured Logging:** Go 1.21+ `log/slog` with JSON output - _Rationale:_ Machine-parseable logs for monitoring tools, consistent context fields, no third-party dependencies

- **Graceful Shutdown:** Signal handling with WaitGroup coordination - _Rationale:_ Prevents message loss during container restarts, demonstrates proper goroutine lifecycle management

**Key Trade-offs:**
- **In-memory buffer only:** 1000 messages lost on application crash (acceptable given container restart speed and rare crash scenarios)
- **No schema migration:** Assumes pre-existing database schema (simplifies app, requires manual DB setup)
- **Fixed retry intervals:** Simple but not optimal for all failure scenarios (good for learning, could be enhanced with exponential backoff)

---

## Tech Stack

This is the **DEFINITIVE technology selection section** - all other documents and agents will reference these choices as the single source of truth.

### Cloud Infrastructure

- **Provider:** Docker-based deployment (infrastructure-agnostic, deployable on any Docker host)
- **Key Services:** None - self-hosted PostgreSQL and Mosquitto via Docker Compose
- **Deployment Regions:** On-premises (home automation infrastructure, Proxmox server)

_Note: This is an on-premises deployment, not cloud-based. The architecture is cloud-ready (can deploy to AWS ECS, GCP Cloud Run, etc.) but targets local infrastructure per PRD requirements._

### Technology Stack Table

| Category | Technology | Version | Purpose | Rationale |
|----------|-----------|---------|---------|-----------|
| **Language** | Go | ~1.23.6 | Primary development language | Latest stable (Feb 2026), native concurrency (goroutines/channels), excellent tooling, strong typing, fast compilation, educational value for learning systems programming |
| **Runtime** | Go Runtime | ~1.23.6 | Application execution environment | Statically-linked binary, minimal dependencies, production-ready |
| **MQTT Client** | Eclipse Paho | ~1.5.0 | MQTT protocol implementation | Official Eclipse Foundation library, production-proven, auto-reconnect support, comprehensive QoS handling, excellent Go integration |
| **Database Driver** | pgx | ~5.7.0 | PostgreSQL connectivity | Modern high-performance driver, superior to lib/pq, native connection pooling, excellent error context, prepared statement support |
| **Database** | PostgreSQL | ~15.10 | Persistent data storage | JSONB native support (metrics column), ACID compliance, mature ecosystem, Grafana integration, widely adopted for time-series data |
| **MQTT Broker** | Mosquitto | ~2.0.20 | Message broker (dev/test only) | Lightweight, standards-compliant MQTT 3.1.1/5.0 broker, official Eclipse project, widely used in IoT |
| **Logging** | log/slog | stdlib (Go 1.23) | Structured logging | Zero dependencies, native JSON output, leveled logging (DEBUG/INFO/ERROR), context-aware, official Go standard as of 1.21 |
| **Configuration** | os.Getenv | stdlib | Environment variable handling | Zero dependencies, twelve-factor app compliance, Docker-native, educational simplicity |
| **Container Base (Builder)** | golang:alpine | ~1.23-alpine3.21 | Multi-stage build (compile stage) | Full Go toolchain, Alpine-based for minimal size |
| **Container Base (Runtime)** | Alpine Linux | ~3.21 | Multi-stage build (runtime stage) | Minimal footprint (~5MB base), security-focused, musl libc compatible with static Go binaries |
| **Code Formatting** | go fmt | stdlib | Code formatting | Official Go formatter, enforces consistent style |
| **Static Analysis** | go vet | stdlib | Basic static analysis | Catches common errors, part of standard toolchain |
| **Linter** | staticcheck | ~2025.1 | Advanced static analysis | Modern industry-standard linter (replaces deprecated golint), catches subtle bugs, go.dev recommended |
| **Testing Framework** | testing | stdlib | Unit/integration testing | Standard Go testing package, table-driven test support, benchmarking, no dependencies - sufficient for project size |
| **Test Coverage** | go test -cover | stdlib | Coverage analysis | Built-in coverage measurement |
| **Build Tool** | go build | stdlib | Binary compilation | Standard Go build tool with ldflags for version injection |
| **Dependency Management** | go modules | stdlib (Go 1.23) | Package management | Official Go dependency management (go.mod/go.sum) |
| **Container Orchestration** | Docker Compose | ~2.24 | Local development & deployment | Multi-container orchestration, simple YAML configuration, sufficient for single-host Proxmox deployment |
| **CI/CD Platform** | GitHub Actions | N/A (SaaS) | Continuous integration | Free for public repos, excellent Go ecosystem support, YAML-based workflows, integrated with GitHub |
| **Container Registry** | Docker Hub | N/A (SaaS) | Container image storage | Free public registry, images primarily built locally on Proxmox server for deployment |
| **Debugger** | Delve (dlv) | ~1.24 | Remote debugging | Official Go debugger, IDE integration, breakpoints/variable inspection |
| **Version Control** | Git | ~2.43 | Source control | Industry standard |

### Version Notation

- **`~X.Y.Z`** notation means: "version X.Y.Z or any newer patch version (X.Y.*)"
  - Example: `~1.5.0` allows `1.5.1`, `1.5.2`, etc. (patch updates) but not `1.6.0` (minor update)
- **`stdlib`** means: tied to Go version (1.23.x)
- **`N/A (SaaS)`** means: cloud service with automatic updates

### Deployment Architecture

**Primary deployment target:** Self-hosted Proxmox server with Docker
- Images built locally on Proxmox using `docker build`
- Docker Hub used as optional backup/distribution channel
- GitHub Actions CI validates builds but production images built on-site
- Eliminates registry pull dependencies during deployment

---

## Data Models

Based on the PRD requirements, MQTT2BDD has a minimal but well-designed data model focused on flexible sensor data capture.

### Model: Message (In-Memory)

**Purpose:** Internal representation of MQTT messages flowing through the application pipeline (channel-based communication between goroutines)

**Key Attributes:**
- **Topic** (`string`) - MQTT topic path identifying the sensor/device (e.g., "zigbee2mqtt/living_room/thermostat")
- **Timestamp** (`time.Time`) - Message receipt timestamp in UTC, used for temporal ordering and analysis
- **Payload** (`[]byte`) - Raw MQTT message payload as byte slice, preserved for flexible JSON storage

**Relationships:**
- No direct relationships - this is a data transfer object (DTO) used internally
- Maps 1:1 to `sensor_metrics` table rows upon persistence

**Design Rationale:**
- **`[]byte` payload:** Preserves raw MQTT data without premature parsing, allows PostgreSQL JSONB to handle schema-less sensor data
- **UTC timestamps:** Ensures consistent temporal analysis across time zones
- **Minimal structure:** Only captures essential fields, avoids over-engineering for this pipeline use case

### Model: sensor_metrics (PostgreSQL Table)

**Purpose:** Persistent storage of all MQTT sensor messages for historical analysis, trending, and Grafana visualization

**Schema Definition:**

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| **id** | BIGINT | GENERATED ALWAYS AS IDENTITY, PRIMARY KEY | Auto-incrementing unique identifier, managed exclusively by PostgreSQL |
| **sensor** | VARCHAR(255) | NOT NULL | MQTT topic path serving as sensor identifier (e.g., "zigbee2mqtt/bedroom/temperature") |
| **date** | TIMESTAMP | NOT NULL | Message timestamp in UTC for time-series analysis |
| **metrics** | JSONB | NOT NULL | Raw JSON payload containing sensor readings (temperature, humidity, state, etc.) |
| | | UNIQUE (sensor, date) | Composite constraint preventing duplicate messages |

**Key Design Decisions:**

1. **IDENTITY vs SERIAL:**
   - Uses modern **SQL:2003 standard** `GENERATED ALWAYS AS IDENTITY` instead of legacy `BIGSERIAL`
   - `ALWAYS` mode prevents manual ID insertion (strict protection against developer errors)
   - Sequence intrinsically tied to column (cleaner lifecycle management)
   - Better portability if database migration needed in future

2. **BIGINT for ID:**
   - Range: -9,223,372,036,854,775,808 to +9,223,372,036,854,775,807
   - At 1 million inserts/day = 25,000+ years before overflow
   - Minimal storage overhead (8 bytes per row)
   - Future-proof for extreme scaling scenarios

3. **UNIQUE(sensor, date) Constraint:**
   - Prevents duplicate message insertion (same sensor + timestamp)
   - TIMESTAMP precision to microsecond makes collisions extremely rare
   - Provides data integrity without application-level deduplication logic
   - INSERT fails with constraint violation if duplicate detected

4. **JSONB (not JSON) for metrics:**
   - Supports efficient indexing on specific JSON keys (e.g., `metrics->>'temperature'`)
   - Binary storage format (faster than text-based JSON)
   - Enables Grafana queries like: `SELECT metrics->>'temperature' FROM sensor_metrics WHERE sensor = 'bedroom/temp'`
   - _Trade-off:_ Slightly larger storage vs JSON, but query performance gain is substantial

**Benefits of ID Column for Future Extensibility:**
- Simple foreign keys: Future tables (alerts, annotations) reference via single `BIGINT` column
- Efficient pagination: Cursor-based with `WHERE id > last_id LIMIT 100`
- Performant JOINs: Integer comparison faster than composite key joins
- Standard tooling: ORMs, admin tools expect single-column PKs

### Data Flow Mapping

```
MQTT Message                    Message Struct                  sensor_metrics Row
-------------                   --------------                  ------------------
topic: "zigbee/bedroom/temp"    Topic: "zigbee/bedroom/temp"   id: 42 (auto-generated)
payload: {"temp": 21.5}    -->  Payload: [bytes]          -->  sensor: "zigbee/bedroom/temp"
timestamp: (broker time)        Timestamp: time.Now()          date: 2026-02-13 14:30:00.123456
                                                               metrics: {"temp": 21.5}
```

**Design Philosophy:**

The data model embraces **schema-on-read** rather than schema-on-write. MQTT devices publish heterogeneous JSON structures (thermostats send `{temperature, humidity}`, switches send `{state}`, energy meters send `{power, voltage, current}`). Storing raw JSONB allows the application to be device-agnostic while enabling Grafana to extract relevant fields at query time.

The addition of an `id` column provides a stable, efficient primary key for future extensibility (alerting, annotations, audit logs) while the `UNIQUE(sensor, date)` constraint maintains data integrity for the time-series use case.

---

## Components

Based on the architectural patterns, tech stack, and monolithic service architecture, MQTT2BDD is organized into focused packages following Go best practices.

### Component: Main Application Coordinator

**Responsibility:** Application lifecycle management, goroutine coordination, signal handling, and graceful shutdown orchestration

**Key Interfaces:**
- Entry point: `func main()` in `cmd/mqtt2bdd/main.go`
- Signal handler: Captures SIGTERM/SIGINT for graceful shutdown
- Goroutine coordinator: Manages MQTT receiver and database writer lifecycles

**Dependencies:**
- All other components (config, logger, mqtt, database)
- Go stdlib: `os/signal`, `context`, `sync`

**Technology Stack:**
- Pure Go stdlib for signal handling and concurrency primitives
- Uses `sync.WaitGroup` for goroutine completion tracking
- Context-based cancellation for coordinated shutdown

**Architecture Notes:**
- Single goroutine responsible for initialization sequence
- Defers cleanup operations (MQTT disconnect, DB close) using Go's defer pattern
- Blocks on signal channel until shutdown requested
- Coordinates flush of buffered messages before exit

### Component: Configuration Manager

**Responsibility:** Load and validate all application configuration from environment variables

**Key Interfaces:**
- `LoadConfig() (*Config, error)` - Reads environment variables, returns populated Config struct or error
- `Config` struct - Holds all application settings (MQTT broker, PostgreSQL DSN, log level, buffer size)

**Dependencies:**
- Go stdlib: `os`, `strconv` (for parsing numeric env vars)

**Technology Stack:**
- Pure stdlib `os.Getenv()` - zero dependencies
- Returns explicit errors for missing required variables

**Configuration Structure:**
```go
type Config struct {
    // MQTT Configuration
    MQTTBroker   string
    MQTTPort     int
    MQTTUsername string // Optional
    MQTTPassword string // Optional

    // PostgreSQL Configuration
    PostgresHost string
    PostgresPort int
    PostgresDB   string
    PostgresUser string
    PostgresPassword string

    // Application Configuration
    LogLevel    string // DEBUG, INFO, ERROR
    BufferSize  int    // Message channel capacity (default: 1000)
}
```

**Validation Rules:**
- Required fields: MQTT broker/port, PostgreSQL connection details
- Optional fields: MQTT auth (for anonymous brokers), BufferSize (default 1000)
- Log level defaults to INFO if invalid/missing

### Component: Structured Logger

**Responsibility:** Provide consistent structured logging across all components with configurable levels

**Key Interfaces:**
- `InitLogger(level string) *slog.Logger` - Initializes and returns configured logger
- Standard `slog.Logger` methods: `Info()`, `Debug()`, `Error()` with key-value context

**Dependencies:**
- Go stdlib: `log/slog`

**Technology Stack:**
- `log/slog` with JSON handler for structured output
- Logs to stdout (Docker captures for centralized logging)
- ISO 8601 timestamps in UTC

**Log Context Fields (Consistent across application):**
- `timestamp` - ISO 8601 UTC
- `level` - DEBUG/INFO/ERROR
- `component` - Package/module name (e.g., "mqtt", "database", "main")
- `message` - Human-readable message
- Additional context: `event`, `error`, `duration_ms`, `topic`, `sensor`, etc.

### Component: MQTT Client

**Responsibility:** Manage MQTT broker connection, subscription to wildcard topics, automatic reconnection, and message reception

**Key Interfaces:**
- `NewClient(config *Config, logger *slog.Logger) *Client` - Constructor
- `Connect(ctx context.Context) error` - Establish connection to broker
- `Subscribe(topic string, handler MessageHandler) error` - Subscribe to topics with callback
- `Disconnect()` - Clean disconnect from broker
- `IsConnected() bool` - Connection health check

**Dependencies:**
- `github.com/eclipse/paho.mqtt.golang` ~1.5.0
- Logger component
- Config component

**Technology Stack:**
- Eclipse Paho MQTT client with QoS 0 (at-most-once delivery)
- Auto-reconnect configuration: 10-second interval
- Connection lost callback for logging and reconnection trigger

**Message Handler Signature:**
```go
type MessageHandler func(topic string, payload []byte)
```

**Reconnection Logic:**
- Paho client handles reconnection automatically with configured interval
- Connection lost callback logs ERROR with reason
- On reconnection, automatically re-subscribes to wildcard (`#`)
- Runs in background goroutine (non-blocking)

**Architecture Notes:**
- Wildcard subscription (`#`) captures all topics without configuration
- Message handler runs in separate goroutine per message (Paho behavior)
- No message acknowledgment (QoS 0) - optimized for throughput over guaranteed delivery

### Component: Database Client

**Responsibility:** Manage PostgreSQL connection pool, execute INSERT operations, automatic reconnection, and connection health monitoring

**Key Interfaces:**
- `NewClient(config *Config, logger *slog.Logger) *Client` - Constructor
- `Connect(ctx context.Context) error` - Establish connection pool
- `InsertMessage(ctx context.Context, sensor string, timestamp time.Time, metrics json.RawMessage) error` - Persist message
- `Close()` - Close connection pool
- `Ping(ctx context.Context) error` - Verify connection health

**Dependencies:**
- `github.com/jackc/pgx/v5/pgxpool` ~5.7.0
- Logger component
- Config component

**Technology Stack:**
- pgx v5 connection pool (min: 1, max: 5 connections)
- Prepared statements for INSERT operations (SQL injection protection)
- Context-based timeouts for all operations

**Connection Pool Configuration:**
```go
poolConfig.MinConns = 1  // Maintain at least one connection
poolConfig.MaxConns = 5  // Sufficient for single writer goroutine
poolConfig.MaxConnIdleTime = 5 * time.Minute
```

**SQL Operations:**
```sql
INSERT INTO sensor_metrics (sensor, date, metrics)
VALUES ($1, $2, $3)
ON CONFLICT (sensor, date) DO NOTHING;  -- Idempotent insert
```

**Reconnection Logic:**
- On write failure due to connection error, log ERROR
- Retry INSERT with 10-second backoff (uses context.WithTimeout)
- Connection verification happens implicitly on successful INSERT
- Failed messages remain in channel buffer (not lost)

**Error Handling:**
- Connection errors: Trigger reconnection loop
- Constraint violations (duplicate): Log WARNING, continue (idempotent)
- Other errors: Log ERROR with full context, continue processing

### Component: Message Buffer (Channel)

**Responsibility:** Decouple MQTT message reception from database writes, provide in-memory buffering during database outages

**Key Interfaces:**
- Buffered channel: `msgChan := make(chan Message, config.BufferSize)`
- Producer: MQTT message handler sends to channel
- Consumer: Database writer goroutine receives from channel

**Dependencies:**
- None (native Go channel)

**Technology Stack:**
- Go buffered channel (native CSP primitive)
- Default capacity: 1000 messages (~5-10 minutes buffer at typical rates)

**Message Structure:**
```go
type Message struct {
    Topic     string
    Timestamp time.Time
    Payload   []byte  // Raw JSON
}
```

**Backpressure Behavior:**
- If channel full (1000 messages), MQTT handler **blocks** until space available
- Prevents memory overflow during prolonged database outages
- Health check logs WARNING when buffer >80% full

**Graceful Shutdown:**
- On SIGTERM, close channel to signal "no more messages"
- Database writer drains all remaining messages before exit
- WaitGroup ensures writer completes before application exits

### Component Diagram

```mermaid
graph TB
    subgraph "Main Goroutine"
        Main[Main Coordinator<br/>Lifecycle & Signals]
        Config[Config Manager<br/>Environment Vars]
        Logger[Structured Logger<br/>slog JSON]
    end

    subgraph "MQTT Goroutine"
        MQTTClient[MQTT Client<br/>Paho ~1.5.0<br/>Subscribe '#']
        MQTTHandler[Message Handler<br/>Extract & Forward]
    end

    subgraph "Message Pipeline"
        Channel[Buffered Channel<br/>1000 capacity<br/>Message struct]
    end

    subgraph "Database Goroutine"
        DBWriter[Database Writer<br/>Consume & Persist]
        DBClient[Database Client<br/>pgx ~5.7.0<br/>Connection Pool]
    end

    subgraph "External Systems"
        Broker[MQTT Broker<br/>Mosquitto]
        Postgres[(PostgreSQL<br/>sensor_metrics)]
    end

    Main -->|initializes| Config
    Main -->|initializes| Logger
    Main -->|starts| MQTTClient
    Main -->|starts| DBWriter
    Config -.->|config| MQTTClient
    Config -.->|config| DBClient
    Logger -.->|logging| MQTTClient
    Logger -.->|logging| DBClient

    Broker -->|messages| MQTTClient
    MQTTClient -->|callback| MQTTHandler
    MQTTHandler -->|send| Channel
    Channel -->|receive| DBWriter
    DBWriter -->|calls| DBClient
    DBClient -->|INSERT| Postgres

    style Main fill:#e1f5ff
    style MQTTClient fill:#fff4e1
    style DBWriter fill:#fff4e1
    style Channel fill:#ffe1e1
    style Config fill:#e8f5e9
    style Logger fill:#e8f5e9
```

### Package Organization (Internal Architecture)

```
cmd/mqtt2bdd/
    main.go                      # Main coordinator - goroutine lifecycle

internal/
    config/
        config.go                # Config struct, LoadConfig()
        config_test.go           # Table-driven tests

    logger/
        logger.go                # InitLogger(), slog wrapper
        logger_test.go           # Log output validation

    mqtt/
        client.go                # MQTT client wrapper (Paho)
        types.go                 # MessageHandler type, Message struct
        client_test.go           # Connection, subscription tests

    database/
        client.go                # PostgreSQL client wrapper (pgx)
        client_test.go           # Connection pool, INSERT tests
```

**Rationale for `internal/` usage:**
- All packages are application-specific (not reusable libraries)
- `internal/` prevents accidental import by other Go modules
- Clear signal: "private implementation, not public API"

### Concurrency Architecture

**Three Goroutines:**

1. **Main Goroutine:**
   - Initializes all components sequentially
   - Blocks on signal channel
   - Coordinates graceful shutdown

2. **MQTT Receiver Goroutine:**
   - Paho client callback runs in library-managed goroutine
   - Message handler sends to channel (blocking if full)
   - Lifecycle managed by Paho library

3. **Database Writer Goroutine:**
   - Infinite loop: `for msg := range msgChan`
   - Blocks when channel empty (efficient waiting)
   - Exits when channel closed (graceful shutdown)

**Synchronization:**
```go
var wg sync.WaitGroup
wg.Add(1)  // Database writer goroutine

// On shutdown:
close(msgChan)         // Signal writer to finish
wg.Wait()              // Wait for writer to drain buffer
dbClient.Close()       // Then close database connection
```

**Design Philosophy:**

The component architecture emphasizes **simplicity and clarity** for educational purposes while maintaining production-grade resilience. Each component has a single, well-defined responsibility with minimal coupling. The goroutine + channel pipeline demonstrates Go's CSP model without over-engineering—no worker pools, no complex synchronization primitives, just clean concurrent data flow.

All components are testable in isolation (dependency injection via constructors) and observable (structured logging at component boundaries).

---

## External APIs

MQTT2BDD is a self-contained bridge application that does not integrate with external REST APIs, SaaS platforms, or third-party web services. The application only connects to infrastructure components:

1. **MQTT Broker (Mosquitto)** - MQTT protocol connection, not HTTP/REST API (covered in Components section)
2. **PostgreSQL Database** - Database protocol connection, not HTTP/REST API (covered in Components section)
3. **Grafana** - Downstream consumer that queries PostgreSQL directly; MQTT2BDD does not call Grafana APIs

**Decision:** No external API integrations required for this project.

---

## Core Workflows

Key system workflows illustrated using sequence diagrams with standardized actor naming and accurate behavior.

### Actor Naming Convention

- **Main** - Main Goroutine (lifecycle coordinator)
- **ConfigMgr** - Configuration Manager
- **Logger** - Structured Logger (slog)
- **MQTTClient** - MQTT Client component (includes Paho goroutine)
- **MsgChannel** - Message Channel (buffered, capacity 1000)
- **DBWriter** - Database Writer Goroutine
- **DBClient** - Database Client (pgx pool wrapper)
- **PostgreSQL** - PostgreSQL database
- **MQTTBroker** - MQTT Broker (Mosquitto)
- **Device** - Zigbee2MQTT device (message source)

### Workflow 1: Normal Operation - Message Capture Flow

```mermaid
sequenceDiagram
    participant Device as Zigbee Device
    participant MQTTBroker as MQTT Broker
    participant MQTTClient as MQTT Client
    participant MsgChannel as Message Channel<br/>(Buffer: 1000)
    participant DBWriter as DB Writer<br/>Goroutine
    participant DBClient as Database Client
    participant PostgreSQL as PostgreSQL

    Note over Device,PostgreSQL: Normal Operation - Single Message Flow

    Device->>MQTTBroker: PUBLISH zigbee2mqtt/bedroom/temp<br/>{"temperature": 21.5}
    MQTTBroker->>MQTTClient: MESSAGE (QoS 0)

    MQTTClient->>MQTTClient: Extract topic, payload<br/>timestamp = time.Now()
    MQTTClient->>Logger: DEBUG: "Message received"
    MQTTClient->>MsgChannel: send Message{topic, ts, payload}
    Note over MsgChannel: Buffered (async)<br/>Count: 1/1000

    MsgChannel->>DBWriter: receive Message
    DBWriter->>DBClient: InsertMessage(sensor, date, metrics)
    DBClient->>PostgreSQL: INSERT INTO sensor_metrics<br/>VALUES ($1, $2, $3)
    PostgreSQL-->>DBClient: Success (1 row)
    DBClient-->>DBWriter: nil (no error)
    DBWriter->>Logger: DEBUG: "Message persisted"<br/>sensor=bedroom/temp

    Note over MQTTClient,DBWriter: Process continues for next message
```

### Workflow 2: Application Startup Sequence

```mermaid
sequenceDiagram
    participant Main as Main Goroutine
    participant ConfigMgr as Config Manager
    participant Logger as Logger
    participant MQTTClient as MQTT Client
    participant DBClient as Database Client
    participant MsgChannel as Message Channel
    participant DBWriter as DB Writer<br/>Goroutine
    participant MQTTBroker as MQTT Broker
    participant PostgreSQL as PostgreSQL

    Note over Main,PostgreSQL: Application Startup Sequence

    Main->>ConfigMgr: LoadConfig()
    ConfigMgr->>ConfigMgr: Read env variables
    ConfigMgr-->>Main: Config{} or error
    alt Config error
        Main->>Main: Log FATAL + Exit(1)
    end

    Main->>Logger: InitLogger(config.LogLevel)
    Logger-->>Main: *slog.Logger
    Main->>Logger: INFO: "MQTT2BDD starting"<br/>version=X.Y.Z

    Main->>MQTTClient: NewClient(config, logger)
    MQTTClient-->>Main: *Client
    Main->>MQTTClient: Connect(ctx)
    MQTTClient->>MQTTBroker: TCP connect + MQTT CONNECT
    MQTTBroker-->>MQTTClient: CONNACK
    MQTTClient-->>Main: nil (success)
    Main->>Logger: INFO: "MQTT connected"<br/>broker=mosquitto:1883

    Main->>DBClient: NewClient(config, logger)
    DBClient-->>Main: *Client
    Main->>DBClient: Connect(ctx)
    DBClient->>PostgreSQL: Create connection pool<br/>(min: 1, max: 5)
    PostgreSQL-->>DBClient: Pool ready
    DBClient-->>Main: nil (success)
    Main->>Logger: INFO: "Database connected"<br/>host=postgres

    Main->>MsgChannel: make(chan Message, 1000)
    Main->>DBWriter: go runDBWriter(msgChan)
    activate DBWriter
    Note over DBWriter: for msg := range msgChan<br/>(blocked waiting)

    Main->>MQTTClient: Subscribe("#", messageHandler)
    MQTTClient->>MQTTBroker: SUBSCRIBE #
    MQTTBroker-->>MQTTClient: SUBACK
    MQTTClient-->>Main: nil (success)
    Main->>Logger: INFO: "Subscribed to all topics"

    Main->>Main: Block on signal channel<br/>sigChan := make(chan os.Signal)

    Note over Main,DBWriter: Application running - processing messages
```

### Workflow 3: Database Outage & Recovery

```mermaid
sequenceDiagram
    participant MQTTClient as MQTT Client
    participant MsgChannel as Message Channel<br/>(1000 capacity)
    participant DBWriter as DB Writer<br/>Goroutine
    participant DBClient as Database Client
    participant PostgreSQL as PostgreSQL
    participant Logger as Logger

    Note over MQTTClient,Logger: Database Outage Scenario

    MQTTClient->>MsgChannel: send Message #1
    MsgChannel->>DBWriter: receive Message #1
    DBWriter->>DBClient: InsertMessage()
    DBClient->>PostgreSQL: INSERT
    PostgreSQL--xDBClient: Connection refused (error)
    DBClient-->>DBWriter: error: connection failed
    DBWriter->>Logger: ERROR: "Database write failed"<br/>error=connection refused

    Note over PostgreSQL: Database offline

    MQTTClient->>MsgChannel: send Message #2
    MQTTClient->>MsgChannel: send Message #3
    MQTTClient->>MsgChannel: send Message #4
    Note over MsgChannel: Messages buffering<br/>Count: 4/1000 (Message #1 being retried)

    DBWriter->>DBWriter: time.Sleep(10 * time.Second)
    DBWriter->>Logger: INFO: "Retrying database write"<br/>attempt=1
    DBWriter->>DBClient: InsertMessage() [retry Message #1]
    DBClient->>PostgreSQL: INSERT
    PostgreSQL--xDBClient: Connection refused
    DBClient-->>DBWriter: error: still down

    MQTTClient->>MsgChannel: send Message #5
    MQTTClient->>MsgChannel: send Message #6
    Note over MsgChannel: Messages buffering<br/>Count: 6/1000

    DBWriter->>DBWriter: time.Sleep(10 * time.Second)
    DBWriter->>Logger: INFO: "Retrying database write"<br/>attempt=2
    DBWriter->>DBClient: InsertMessage() [retry Message #1]

    Note over PostgreSQL: Database back online

    DBClient->>PostgreSQL: INSERT
    PostgreSQL-->>DBClient: Success (1 row)
    DBClient-->>DBWriter: nil (success)
    DBWriter->>Logger: INFO: "Database reconnected"
    DBWriter->>Logger: DEBUG: "Message persisted (retried)"

    MsgChannel->>DBWriter: receive Message #2
    DBWriter->>DBClient: InsertMessage()
    DBClient->>PostgreSQL: INSERT
    PostgreSQL-->>DBClient: Success

    MsgChannel->>DBWriter: receive Message #3
    DBWriter->>DBClient: InsertMessage()
    DBClient->>PostgreSQL: INSERT
    PostgreSQL-->>DBClient: Success

    Note over DBWriter,PostgreSQL: Continue draining buffer<br/>until channel empty
```

### Workflow 4: Graceful Shutdown

```mermaid
sequenceDiagram
    participant OS as OS Signal<br/>(SIGTERM/SIGINT)
    participant Main as Main Goroutine
    participant MQTTClient as MQTT Client
    participant MsgChannel as Message Channel
    participant DBWriter as DB Writer<br/>Goroutine
    participant DBClient as Database Client
    participant PostgreSQL as PostgreSQL
    participant Logger as Logger

    Note over OS,Logger: Graceful Shutdown - docker stop

    OS->>Main: SIGTERM received<br/>(signal channel)
    Main->>Logger: INFO: "Shutdown signal received"

    Main->>MQTTClient: Disconnect()
    MQTTClient->>MQTTClient: Unsubscribe from topics<br/>Close broker connection
    MQTTClient-->>Main: Done
    Main->>Logger: INFO: "MQTT disconnected"

    Main->>MsgChannel: close(msgChan)
    Note over MsgChannel: Closed - no more sends allowed<br/>Existing messages preserved<br/>Count: 3 messages remaining

    Note over DBWriter: for msg := range msgChan<br/>(draining buffer)

    MsgChannel->>DBWriter: receive Message #1
    DBWriter->>DBClient: InsertMessage()
    DBClient->>PostgreSQL: INSERT
    PostgreSQL-->>DBClient: Success

    MsgChannel->>DBWriter: receive Message #2
    DBWriter->>DBClient: InsertMessage()
    DBClient->>PostgreSQL: INSERT
    PostgreSQL-->>DBClient: Success

    MsgChannel->>DBWriter: receive Message #3
    DBWriter->>DBClient: InsertMessage()
    DBClient->>PostgreSQL: INSERT
    PostgreSQL-->>DBClient: Success

    Note over MsgChannel: Channel empty & closed

    DBWriter->>DBWriter: Exit for loop<br/>(channel closed & drained)
    DBWriter->>Main: Signal completion<br/>(via goroutine return)

    Main->>Main: Wait for DBWriter to finish<br/>(blocking via WaitGroup)
    Main->>Logger: INFO: "Flushed 3 messages"
    Main->>DBClient: Close()
    DBClient->>PostgreSQL: Close connection pool
    PostgreSQL-->>DBClient: Closed
    Main->>Logger: INFO: "Shutdown complete"
    Main->>Main: os.Exit(0)
```

### Workflow 5: MQTT Broker Disconnection & Reconnection

```mermaid
sequenceDiagram
    participant MQTTBroker as MQTT Broker
    participant MQTTClient as MQTT Client
    participant Logger as Logger
    participant MsgChannel as Message Channel
    participant DBWriter as DB Writer<br/>Goroutine
    participant DBClient as Database Client

    Note over MQTTBroker,DBClient: MQTT Broker Outage

    MQTTBroker--xMQTTClient: Connection lost<br/>(network failure)
    MQTTClient->>MQTTClient: onConnectionLost callback<br/>(configured in Paho)
    MQTTClient->>Logger: ERROR: "MQTT connection lost"<br/>reason=network error

    Note over MQTTClient: Paho auto-reconnect active<br/>(10 second interval configured)

    Note over MsgChannel,DBClient: Database writer continues<br/>processing existing buffer

    MsgChannel->>DBWriter: receive buffered Message
    DBWriter->>DBClient: InsertMessage()
    DBClient->>DBClient: Continue normal processing<br/>(MQTT outage doesn't block DB)

    MQTTClient->>MQTTClient: time.Sleep(10 * time.Second)
    MQTTClient->>Logger: INFO: "Attempting MQTT reconnection"<br/>attempt=1
    MQTTClient->>MQTTBroker: TCP connect + MQTT CONNECT
    MQTTBroker--xMQTTClient: Connection refused
    MQTTClient->>Logger: WARN: "Reconnection failed"

    MQTTClient->>MQTTClient: time.Sleep(10 * time.Second)
    MQTTClient->>Logger: INFO: "Attempting MQTT reconnection"<br/>attempt=2
    MQTTClient->>MQTTBroker: TCP connect + MQTT CONNECT

    Note over MQTTBroker: Broker back online

    MQTTBroker-->>MQTTClient: CONNACK
    MQTTClient->>Logger: INFO: "MQTT reconnected"

    MQTTClient->>MQTTBroker: SUBSCRIBE # (wildcard)
    MQTTBroker-->>MQTTClient: SUBACK
    MQTTClient->>Logger: INFO: "Resubscribed to all topics"

    Note over MQTTClient,MsgChannel: Resume message reception
    MQTTBroker->>MQTTClient: New messages
    MQTTClient->>MsgChannel: send Messages
```

### Buffering Limits & Message Loss Scenarios

**Critical Understanding: Multiple Buffer Layers**

```
Zigbee Device → MQTT Broker → [Paho Internal Buffer] → [App Message Channel] → Database
                                 (configurable)           (1000 capacity)
```

**Buffer Configuration:**
```go
opts := mqtt.NewClientOptions()
opts.SetMessageChannelDepth(5000)  // Paho internal buffer (default: 100)

msgChan := make(chan Message, 1000)  // Application channel
```

**Message Loss Risk Matrix:**

| Scenario | Duration | Buffering Capacity | Message Loss? |
|----------|----------|-------------------|---------------|
| DB outage | < 5 min | App channel sufficient | **No** |
| DB outage | 5-10 min | App + Paho buffers | **No** (if rate < 10 msg/sec) |
| DB outage | > 10 min | Both buffers overflow | **Yes** |
| MQTT outage | Any | N/A - source unavailable | **Yes** (messages never received) |
| App crash (SIGKILL) | Instant | Buffers lost | **Yes** (in-memory only) |
| Graceful shutdown | < 30 sec | Buffers flushed | **No** |

### Error Handling Summary

| Error Scenario | Behavior | Recovery | Data Loss? |
|---------------|----------|----------|------------|
| **Startup failure** (config/connection) | Log FATAL, Exit(1) | Manual intervention required | N/A (app never started) |
| **Database outage** (< buffer capacity) | Buffer messages in channel, retry every 10s | Automatic when DB recovers | **No** |
| **Database outage** (> buffer capacity) | Both buffers full, Paho drops new messages | Automatic reconnect, but messages lost | **Yes** (overflow messages) |
| **MQTT outage** | Auto-reconnect every 10s, resubscribe on connect | Automatic when broker recovers | **Yes** (messages during outage never received) |
| **Channel full** (DB very slow) | MQTT handler blocks, Paho buffers internally | Unblocks when DB writer drains channel | **Depends** (no if Paho buffer sufficient, yes if Paho also fills) |
| **Duplicate message** | UNIQUE constraint violation on (sensor, date) | Log WARNING, skip (idempotent via ON CONFLICT) | **No** (duplicate rejected) |
| **Graceful shutdown** (SIGTERM) | Drain both buffers before exit (< 30s) | Clean shutdown with flush | **No** |
| **Forced kill** (SIGKILL) | Immediate termination, no cleanup | None | **Yes** (all buffered messages lost) |

**Key Takeaway:**

The system provides **best-effort durability** with ~5-10 minutes of buffer capacity during database outages (depending on message rate). This is sufficient for typical transient failures (restarts, network blips) but **not guaranteed storage**. For critical applications requiring zero message loss, additional persistence layers (disk-based queues, MQTT QoS 1/2) would be needed—trade-offs consciously avoided for this learning-focused project.

---

## REST API Spec

**Condition:** Project includes REST API

**Analysis:** MQTT2BDD does not expose a REST API. The application is a headless data pipeline service with no HTTP endpoints. Data access is performed directly via PostgreSQL queries (Grafana or other tools).

**Decision:** This section is not applicable.

---

## Database Schema

Transform the conceptual data model into concrete PostgreSQL schema with DDL statements.

### Database: PostgreSQL ~15.10

**Schema Name:** `public` (default schema)

**Database Name:** Configurable via environment variable `POSTGRES_DB` (example: `mqtt2bdd`)

### PostgreSQL vs TimescaleDB Decision

**Evaluated Options:**

**Option A: PostgreSQL Standard (Chosen)**
- Native time-series capability (TIMESTAMP + indexes)
- Zero external dependencies
- Simpler operations and maintenance
- Proven for volumes < 1 billion rows
- Better for learning objectives

**Option B: TimescaleDB Extension**
- Automatic time-based partitioning (hypertables)
- Native compression (90%+ space savings)
- Continuous aggregates (auto-refreshing materialized views)
- Optimized for extreme scale (> 1 billion rows)
- Additional operational complexity

**Decision: PostgreSQL Standard**

**Rationale:**
- **Projected volume:** ~50M rows/year (100 devices × 1 msg/min) = well within PostgreSQL capacity
- **Query patterns:** Simple time-range SELECTs (Grafana) - no complex time-series analytics
- **Educational goal:** Minimize external dependencies, focus on Go learning
- **Operational simplicity:** No extension management, no version compatibility issues

**Migration path:** If volume exceeds 500M rows or storage becomes constrained, migrate to TimescaleDB (schema changes minimal)

### Table: sensor_metrics

**DDL Statement:**

```sql
-- Create table with IDENTITY primary key and composite unique constraint
CREATE TABLE sensor_metrics (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sensor VARCHAR(255) NOT NULL,
    date TIMESTAMP NOT NULL,
    metrics JSONB NOT NULL,
    CONSTRAINT unique_sensor_date UNIQUE (sensor, date)
);

-- Create composite index for typical Grafana queries (sensor + date range)
CREATE INDEX idx_sensor_metrics_sensor_date ON sensor_metrics (sensor, date DESC);

-- Create GIN index for JSONB queries (filtering on JSON fields)
CREATE INDEX idx_sensor_metrics_metrics ON sensor_metrics USING GIN (metrics);
```

**Column Details:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | BIGINT | NOT NULL | IDENTITY | Auto-incrementing unique identifier (SQL:2003 standard), managed exclusively by PostgreSQL |
| `sensor` | VARCHAR(255) | NOT NULL | - | MQTT topic path serving as sensor identifier (e.g., "zigbee2mqtt/bedroom/temperature") |
| `date` | TIMESTAMP | NOT NULL | - | Message timestamp in UTC with microsecond precision for time-series ordering |
| `metrics` | JSONB | NOT NULL | - | Raw JSON payload containing sensor readings in binary format for efficient querying |

**Indexes:**

1. **Primary Key Index (Automatic):** B-tree index on `id` column
2. **Composite Index:** `idx_sensor_metrics_sensor_date` - B-tree index on `(sensor, date DESC)`
3. **GIN Index:** `idx_sensor_metrics_metrics` - Generalized Inverted Index on `metrics` JSONB column
   - **Trade-off:** Index ~30% of data size, INSERTs ~10% slower, JSON queries 10-100x faster

### Schema Initialization

**Location:** `dev/init-db/01-schema.sql`

```sql
-- MQTT2BDD Database Schema
-- Version: 1.0
-- Purpose: Time-series storage for MQTT sensor messages

-- Create sensor_metrics table
CREATE TABLE IF NOT EXISTS sensor_metrics (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sensor VARCHAR(255) NOT NULL,
    date TIMESTAMP NOT NULL,
    metrics JSONB NOT NULL,
    CONSTRAINT unique_sensor_date UNIQUE (sensor, date)
);

-- Create composite index for time-series queries
CREATE INDEX IF NOT EXISTS idx_sensor_metrics_sensor_date
ON sensor_metrics (sensor, date DESC);

-- Create GIN index for JSONB queries
CREATE INDEX IF NOT EXISTS idx_sensor_metrics_metrics
ON sensor_metrics USING GIN (metrics);

-- Display table info
\d sensor_metrics

-- Display indexes
\di sensor_metrics*
```

### Application INSERT Operations

**Standard Insert (Go code with duplicate detection):**

```go
func (c *Client) InsertMessage(ctx context.Context, sensor string, timestamp time.Time, metrics json.RawMessage) error {
    query := `
        INSERT INTO sensor_metrics (sensor, date, metrics)
        VALUES ($1, $2, $3)
        ON CONFLICT (sensor, date) DO NOTHING
    `

    // Use Exec which returns CommandTag with rows affected
    commandTag, err := c.pool.Exec(ctx, query, sensor, timestamp, metrics)
    if err != nil {
        return fmt.Errorf("failed to insert message: %w", err)
    }

    // Check if row was actually inserted (0 = conflict occurred)
    rowsAffected := commandTag.RowsAffected()
    if rowsAffected == 0 {
        c.logger.Warn("Duplicate message ignored (conflict on sensor+date)",
            "sensor", sensor,
            "timestamp", timestamp.Format(time.RFC3339),
        )
    } else {
        c.logger.Debug("Message persisted",
            "sensor", sensor,
            "timestamp", timestamp.Format(time.RFC3339),
            "payload_size", len(metrics),
        )
    }

    return nil
}
```

**Key Features:**
- **Parameterized query:** Prevents SQL injection
- **ON CONFLICT DO NOTHING:** Idempotent writes
- **RowsAffected() check:** Detects duplicates (0 = conflict, 1 = inserted)
- **Conditional logging:** DEBUG for success, WARN for duplicates

### Storage Estimates

| Metric | Value | Calculation |
|--------|-------|-------------|
| Messages/day | 144,000 | 100 devices × 60 min × 24 hours |
| Messages/year | 52,560,000 | 144,000 × 365 |
| Row size | ~300 bytes | 8 (id) + ~30 (sensor) + 8 (date) + 200 (metrics) + overhead |
| Storage/year | ~15 GB | 52.5M rows × 300 bytes |
| Index size (B-tree) | ~3 GB | Composite index |
| Index size (GIN) | ~5 GB | GIN index on JSONB |
| **Total/year** | **~23 GB** | Data + indexes |

**5-Year Projection:** ~115 GB

### Example Queries (Grafana)

**Query 1: Latest 1000 readings**
```sql
SELECT date, metrics
FROM sensor_metrics
WHERE sensor = 'zigbee2mqtt/bedroom/thermostat'
ORDER BY date DESC
LIMIT 1000;
```

**Query 2: Temperature trend (24 hours)**
```sql
SELECT
    date,
    (metrics->>'temperature')::numeric AS temperature
FROM sensor_metrics
WHERE sensor = 'zigbee2mqtt/bedroom/thermostat'
  AND date > NOW() - INTERVAL '24 hours'
ORDER BY date ASC;
```

**Query 3: Hourly average**
```sql
SELECT
    date_trunc('hour', date) AS hour,
    AVG((metrics->>'temperature')::numeric) AS avg_temp
FROM sensor_metrics
WHERE sensor = 'zigbee2mqtt/bedroom/thermostat'
  AND date > NOW() - INTERVAL '7 days'
GROUP BY hour
ORDER BY hour ASC;
```

### Index Maintenance

**VACUUM ANALYZE (Updates statistics):**
- **Frequency:** Monthly (optional - autovacuum handles most cases)

```bash
# Monthly cron job (first day of month at 3 AM)
0 3 1 * * docker exec mqtt2bdd-prod-postgres psql -U user -d mqtt2bdd -c "VACUUM ANALYZE sensor_metrics;"
```

**REINDEX (Rebuilds indexes):**
- **Frequency:** Quarterly (optional for INSERT-only tables)

```bash
# Quarterly cron job (first day of quarter at 3 AM)
0 3 1 */3 * docker exec mqtt2bdd-prod-postgres psql -U user -d mqtt2bdd -c "REINDEX TABLE CONCURRENTLY sensor_metrics;"
```

**Note:** With `autovacuum = on` (default), manual maintenance is largely optional.

### Performance Tuning

**Recommended `postgresql.conf` settings:**

```ini
# Memory settings (for 8GB RAM server)
shared_buffers = 2GB
effective_cache_size = 6GB
work_mem = 64MB

# Write-heavy optimizations
wal_buffers = 16MB
checkpoint_completion_target = 0.9
max_wal_size = 2GB

# Autovacuum tuning
autovacuum = on
autovacuum_max_workers = 2
autovacuum_naptime = 30s
```

---

## Source Tree

Project folder structure reflecting the monorepo organization, Go project layout best practices, and containerized development/deployment environments.

### Complete Project Structure

```
mqtt2bdd/
├── .github/
│   └── workflows/
│       ├── ci.yml                          # GitHub Actions CI pipeline
│       └── release.yml                     # Release automation
│
├── .vscode/
│   ├── launch.json                         # Delve remote debugging config
│   └── settings.json                       # Go extension settings
│
├── cmd/
│   └── mqtt2bdd/
│       └── main.go                         # Application entry point
│
├── internal/
│   ├── config/
│   │   ├── config.go                       # Config struct, LoadConfig()
│   │   └── config_test.go                  # Table-driven tests
│   │
│   ├── logger/
│   │   ├── logger.go                       # InitLogger(), slog wrapper
│   │   └── logger_test.go                  # Logger tests
│   │
│   ├── mqtt/
│   │   ├── client.go                       # MQTT client wrapper (Paho)
│   │   ├── types.go                        # Message struct, MessageHandler
│   │   └── client_test.go                  # Connection tests
│   │
│   └── database/
│       ├── client.go                       # PostgreSQL client wrapper (pgx)
│       ├── queries.go                      # InsertMessage() and operations
│       └── client_test.go                  # Connection pool, INSERT tests
│
├── pkg/
│   └── (empty - for future reusable libraries)
│
├── dev/
│   ├── docker-compose.yml                  # Dev environment (3 containers)
│   ├── .env.example                        # Template for env variables
│   ├── init-db/
│   │   └── 01-schema.sql                   # Database schema init
│   └── mosquitto/
│       └── mosquitto.conf                  # Mosquitto config
│
├── test/
│   ├── docker-compose.yml                  # Isolated test environment
│   ├── init-db/
│   │   └── 01-schema.sql                   # Test database schema
│   └── run-integration-tests.sh            # Integration test runner
│
├── scripts/
│   ├── build.sh                            # Build with version injection
│   ├── run-tests.sh                        # Run tests with coverage
│   └── lint.sh                             # Run staticcheck + go vet
│
├── docs/
│   ├── architecture.md                     # This document
│   ├── prd.md                              # Product Requirements Document
│   └── development-guide.md                # Setup, debugging guide
│
├── .gitignore                              # Git ignore patterns
├── .dockerignore                           # Docker ignore patterns
├── Dockerfile                              # Production multi-stage build
├── docker-compose.prod.yml                 # Production deployment stack
├── go.mod                                  # Go module definition
├── go.sum                                  # Go module checksums
├── Makefile                                # Common tasks automation
├── README.md                               # Project overview
└── LICENSE                                 # License file
```

### Package Import Paths

**Go Module Path:** `github.com/username/mqtt2bdd`

**Internal Imports:**
```go
import (
    "github.com/username/mqtt2bdd/internal/config"
    "github.com/username/mqtt2bdd/internal/logger"
    "github.com/username/mqtt2bdd/internal/mqtt"
    "github.com/username/mqtt2bdd/internal/database"
)
```

**External Dependencies (go.mod):**
```go
module github.com/username/mqtt2bdd

go 1.23

require (
    github.com/eclipse/paho.mqtt.golang v1.5.0
    github.com/jackc/pgx/v5 v5.7.2
)
```

### Design Rationale

**Follows Go Project Layout Best Practices:**
- `cmd/` for executables (one subdirectory per binary)
- `internal/` for private packages (enforced by Go compiler)
- `pkg/` for public libraries (empty until needed)
- Root-level config files (go.mod, Dockerfile, Makefile)

**Separates Environments:**
- `dev/` for rapid development (hot reload, Delve debugging)
- `test/` for isolated integration testing (separate ports, ephemeral data)
- Production Dockerfile and docker-compose.prod.yml at root

**Educational Structure:**
- Clear package boundaries (config, logger, mqtt, database)
- Consistent naming conventions
- Well-commented file purposes
- Scripts for common tasks

---

## Infrastructure and Deployment

Defining the deployment architecture for MQTT2BDD's on-premises Proxmox deployment with containerized development environments.

### Infrastructure as Code

**Tool:** Docker Compose ~2.24

**Location:**
- Development: `dev/docker-compose.yml`
- Testing: `test/docker-compose.yml`
- Production: `docker-compose.prod.yml` (root directory)

**Approach:** Declarative YAML-based container orchestration

**Rationale:**
- **No cloud provider:** On-premises Proxmox deployment doesn't require Terraform/CloudFormation
- **Single-host deployment:** Docker Compose sufficient (no Kubernetes overhead)
- **Simplicity:** YAML configs are human-readable and version-controlled
- **Portability:** Can migrate to cloud (AWS ECS, GCP Cloud Run) by converting Compose files
- **Educational value:** Straightforward infrastructure-as-code for learning

**Alternative Considered:**
- **Kubernetes/K3s:** Rejected - overkill for single application, adds operational complexity
- **Terraform:** Rejected - no cloud resources to provision, Proxmox has native Docker support
- **Ansible:** Could be added later for multi-host Proxmox clusters, not needed now

### Deployment Strategy

**Strategy:** Rolling Update with minimal downtime

**CI/CD Platform:** GitHub Actions (free for public repos)

**Pipeline Configuration:** `.github/workflows/ci.yml` and `.github/workflows/release.yml`

**Deployment Flow:**

```
Developer Push → GitHub → GitHub Actions CI
                              ↓
                    [Build + Test + Lint]
                              ↓
                    Git Tag (vX.Y.Z)
                              ↓
                    GitHub Actions Release
                              ↓
              [Build Docker Image + Push to Docker Hub]
                              ↓
                    Manual Deploy to Proxmox
                              ↓
              [Pull Image + docker-compose up]
```

**Detailed Deployment Steps:**

1. **CI Pipeline** (`.github/workflows/ci.yml`) - Runs on every push/PR:
   ```yaml
   name: CI
   on: [push, pull_request]
   jobs:
     test:
       runs-on: ubuntu-latest
       steps:
         - uses: actions/checkout@v4
         - uses: actions/setup-go@v5
           with:
             go-version: '~1.23'
         - name: Format check
           run: go fmt ./... && git diff --exit-code
         - name: Vet
           run: go vet ./...
         - name: Staticcheck
           run: |
             go install honnef.co/go/tools/cmd/staticcheck@latest
             staticcheck ./...
         - name: Tests
           run: go test -v -cover ./...
         - name: Build
           run: go build -o bin/mqtt2bdd ./cmd/mqtt2bdd
   ```

2. **Release Pipeline** (`.github/workflows/release.yml`) - Runs on git tags:
   ```yaml
   name: Release
   on:
     push:
       tags:
         - 'v*'
   jobs:
     release:
       runs-on: ubuntu-latest
       steps:
         - uses: actions/checkout@v4
         - name: Extract version
           id: version
           run: echo "VERSION=${GITHUB_REF#refs/tags/v}" >> $GITHUB_OUTPUT
         - name: Build Docker image
           run: |
             docker build \
               --build-arg VERSION=${{ steps.version.outputs.VERSION }} \
               -t mqtt2bdd:${{ steps.version.outputs.VERSION }} \
               -t mqtt2bdd:latest \
               .
         - name: Login to Docker Hub
           uses: docker/login-action@v3
           with:
             username: ${{ secrets.DOCKERHUB_USERNAME }}
             password: ${{ secrets.DOCKERHUB_TOKEN }}
         - name: Push to Docker Hub
           run: |
             docker tag mqtt2bdd:${{ steps.version.outputs.VERSION }} \
               ${{ secrets.DOCKERHUB_USERNAME }}/mqtt2bdd:${{ steps.version.outputs.VERSION }}
             docker tag mqtt2bdd:latest \
               ${{ secrets.DOCKERHUB_USERNAME }}/mqtt2bdd:latest
             docker push ${{ secrets.DOCKERHUB_USERNAME }}/mqtt2bdd:${{ steps.version.outputs.VERSION }}
             docker push ${{ secrets.DOCKERHUB_USERNAME }}/mqtt2bdd:latest
         - name: Create GitHub Release
           uses: softprops/action-gh-release@v1
           with:
             generate_release_notes: true
   ```

3. **Production Deployment** (Manual on Proxmox):
   ```bash
   # SSH to Proxmox host
   ssh user@proxmox-server

   # Navigate to project directory
   cd /opt/mqtt2bdd

   # Pull latest code (or specific tag)
   git pull origin main
   # OR: git checkout v0.1.0

   # Option A: Build locally (preferred for on-premises)
   docker build --build-arg VERSION=0.1.0 -t mqtt2bdd:0.1.0 .

   # Option B: Pull from Docker Hub (if built in CI)
   # docker pull username/mqtt2bdd:0.1.0

   # Deploy with Docker Compose (rolling update)
   docker-compose -f docker-compose.prod.yml up -d

   # Docker Compose automatically:
   # - Stops old container gracefully (SIGTERM)
   # - Starts new container with new image
   # - Minimal downtime (~10-15 seconds)

   # Verify deployment
   docker-compose -f docker-compose.prod.yml ps
   docker-compose -f docker-compose.prod.yml logs -f mqtt2bdd
   ```

**Why Not Blue-Green Deployment?**

Blue-Green deployment is **not suitable** for MQTT2BDD:

**Problem:** MQTT wildcard subscription model
- Both instances (blue + green) subscribe to `#` (all topics)
- MQTT broker delivers each message to **both subscribers**
- Result: **Message duplication** - each message written twice to PostgreSQL
- Even with `UNIQUE(sensor, date)`, microsecond timestamp differences = duplicates

**Chosen Approach:** Rolling Update with acceptable downtime
- Docker Compose replaces old container with new one (`up -d`)
- Graceful shutdown: SIGTERM → 10s buffer flush → SIGKILL
- Expected downtime: **10-15 seconds**
- Messages published during downtime are lost (acceptable for home automation with QoS 0)
- Mitigation: Deploy during low-activity hours (e.g., 3 AM)

**Rationale:**
- **Manual deployment to Proxmox:** Prevents accidental production deployments, appropriate for single-host home infrastructure
- **GitHub Actions for CI:** Automated testing catches regressions before merge
- **Local builds preferred:** Eliminates registry pull dependencies, faster on local network
- **Docker Hub as backup:** Optional image distribution for multi-host future scenarios

### Environments

**1. Development Environment** (`dev/docker-compose.yml`)

**Purpose:** Rapid local development with hot-reload and debugging support

**Details:**
- **Containers:**
  - `mqtt2bdd-dev-postgres` (PostgreSQL 15-alpine)
  - `mqtt2bdd-dev-mosquitto` (Mosquitto 2.0)
  - `mqtt2bdd-dev-go-dev` (Go 1.23-alpine + Delve)
- **Network:** `mqtt2bdd-dev-network` (isolated bridge network)
- **Volumes:**
  - Project directory mounted at `/app` (live code editing)
  - PostgreSQL data persisted in named volume `mqtt2bdd-dev-pgdata`
  - Mosquitto config mounted from `dev/mosquitto/`
- **Ports Exposed:**
  - PostgreSQL: `5432:5432`
  - Mosquitto: `1883:1883`
  - Delve Debugger: `2345:2345`
- **Configuration:** `.env` file (from `.env.example` template)
- **Startup:** `cd dev/ && docker-compose up`

**Access:**
```bash
# Start dev environment
cd dev/
cp .env.example .env
docker-compose up

# Execute commands in Go dev container
docker exec -it mqtt2bdd-dev-go-dev sh
go run ./cmd/mqtt2bdd

# Attach debugger (VS Code launch.json configured)
# Set breakpoints, press F5 in VS Code

# Publish test MQTT message
docker exec mqtt2bdd-dev-mosquitto mosquitto_pub -t 'test/temp' -m '{"value": 22.5}'

# Query database
docker exec -it mqtt2bdd-dev-postgres psql -U mqtt2bdd -d mqtt2bdd -c "SELECT * FROM sensor_metrics LIMIT 10;"
```

---

**2. Test Environment** (`test/docker-compose.yml`)

**Purpose:** Isolated integration testing with ephemeral data

**Details:**
- **Containers:**
  - `mqtt2bdd-test-postgres` (PostgreSQL 15-alpine)
  - `mqtt2bdd-test-mosquitto` (Mosquitto 2.0)
  - `mqtt2bdd-test-app` (Application under test)
- **Network:** `mqtt2bdd-test-network` (isolated, no conflicts with dev)
- **Ports:** **No ports exposed to host** - containers communicate via internal Docker DNS
  - PostgreSQL accessible at `mqtt2bdd-test-postgres:5432` (internal)
  - Mosquitto accessible at `mqtt2bdd-test-mosquitto:1883` (internal)
- **Volumes:** Ephemeral (no persistence - fresh state per test run)
- **Startup:** Automated via `test/run-integration-tests.sh`

**Test Docker Compose Configuration:**
```yaml
version: '3.8'

services:
  postgres:
    container_name: mqtt2bdd-test-postgres
    image: postgres:15-alpine
    environment:
      POSTGRES_DB: mqtt2bdd
      POSTGRES_USER: mqtt2bdd
      POSTGRES_PASSWORD: test_password
    volumes:
      - ./init-db:/docker-entrypoint-initdb.d:ro
    networks:
      - mqtt2bdd-test-network
    # No ports exposed - internal only

  mosquitto:
    container_name: mqtt2bdd-test-mosquitto
    image: eclipse-mosquitto:2
    volumes:
      - ../dev/mosquitto/mosquitto.conf:/mosquitto/config/mosquitto.conf:ro
    networks:
      - mqtt2bdd-test-network
    # No ports exposed - internal only

  app:
    container_name: mqtt2bdd-test-app
    build:
      context: ..
      dockerfile: Dockerfile
    depends_on:
      - postgres
      - mosquitto
    environment:
      MQTT_BROKER: mqtt2bdd-test-mosquitto
      MQTT_PORT: 1883
      POSTGRES_HOST: mqtt2bdd-test-postgres
      POSTGRES_PORT: 5432
      POSTGRES_DB: mqtt2bdd
      POSTGRES_USER: mqtt2bdd
      POSTGRES_PASSWORD: test_password
      LOG_LEVEL: DEBUG
    networks:
      - mqtt2bdd-test-network

networks:
  mqtt2bdd-test-network:
    driver: bridge
```

**Integration Test Script:**
```bash
#!/bin/bash
set -e

echo "Starting test environment..."
cd test/
docker-compose up -d

echo "Waiting for services to be ready..."
sleep 10

echo "Running integration tests..."
docker-compose exec -T mqtt2bdd-test-app go test -v ./... -tags=integration

echo "Testing MQTT → PostgreSQL flow..."
docker-compose exec -T mqtt2bdd-test-mosquitto \
  mosquitto_pub -t 'integration/test' -m '{"test_value": 42}'

sleep 2

echo "Verifying database persistence..."
ROWS=$(docker-compose exec -T mqtt2bdd-test-postgres \
  psql -U mqtt2bdd -d mqtt2bdd -t -c "SELECT COUNT(*) FROM sensor_metrics WHERE sensor='integration/test';")

if [ "$ROWS" -ge 1 ]; then
  echo "✅ Integration test passed: Message persisted to database"
else
  echo "❌ Integration test failed: Message not found in database"
  exit 1
fi

echo "Cleaning up test environment..."
docker-compose down -v

echo "✅ All integration tests passed!"
```

**Run Tests:**
```bash
./test/run-integration-tests.sh
```

**Benefits of No Exposed Ports:**
- No port conflicts between dev/ and test/ environments running concurrently
- Complete isolation (no external access to test services)
- Simpler configuration
- Test script uses `docker-compose exec` to interact with containers

---

**3. Production Environment** (`docker-compose.prod.yml`)

**Purpose:** Production deployment on Proxmox server

**Critical Assumption:** PostgreSQL and Mosquitto are **already deployed externally** on Proxmox infrastructure (managed separately from this project).

**Details:**
- **Containers:** `mqtt2bdd-prod-app` (Application only)
- **External Services:**
  - PostgreSQL server accessible via IP/hostname (e.g., `postgres.local:5432`)
  - Mosquitto broker accessible via IP/hostname (e.g., `mosquitto.local:1883`)
- **Network:** Uses default Docker bridge or host network to reach external services
- **Restart Policy:** `unless-stopped` (auto-restart on failure/reboot)
- **Resource Limits:**
  ```yaml
  deploy:
    resources:
      limits:
        cpus: '0.5'
        memory: 256M
      reservations:
        cpus: '0.25'
        memory: 128M
  ```
- **Logging:**
  ```yaml
  logging:
    driver: "json-file"
    options:
      max-size: "10m"
      max-file: "3"
  ```

**Production docker-compose.prod.yml:**
```yaml
version: '3.8'

services:
  app:
    container_name: mqtt2bdd-prod-app
    image: mqtt2bdd:${VERSION:-latest}
    restart: unless-stopped
    environment:
      # MQTT Broker (external - already deployed on Proxmox)
      MQTT_BROKER: ${MQTT_BROKER:-mosquitto.local}
      MQTT_PORT: ${MQTT_PORT:-1883}
      MQTT_USERNAME: ${MQTT_USERNAME:-}
      MQTT_PASSWORD: ${MQTT_PASSWORD:-}

      # PostgreSQL (external - already deployed on Proxmox)
      POSTGRES_HOST: ${POSTGRES_HOST:-postgres.local}
      POSTGRES_PORT: ${POSTGRES_PORT:-5432}
      POSTGRES_DB: ${POSTGRES_DB:-mqtt2bdd}
      POSTGRES_USER: ${POSTGRES_USER:-mqtt2bdd}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}

      # Application
      LOG_LEVEL: ${LOG_LEVEL:-INFO}
      BUFFER_SIZE: ${BUFFER_SIZE:-1000}

    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"

    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 256M
        reservations:
          cpus: '0.25'
          memory: 128M
```

**Production Environment Variables** (`.env.prod`)
```bash
# Version de l'application
VERSION=0.1.0

# MQTT Broker externe (hostname ou IP)
MQTT_BROKER=mosquitto.local
MQTT_PORT=1883
MQTT_USERNAME=
MQTT_PASSWORD=

# PostgreSQL externe (hostname ou IP)
POSTGRES_HOST=postgres.local
POSTGRES_PORT=5432
POSTGRES_DB=mqtt2bdd
POSTGRES_USER=mqtt2bdd
POSTGRES_PASSWORD=<strong_password>

# Application
LOG_LEVEL=INFO
BUFFER_SIZE=1000
```

**External Services Prerequisites (Proxmox Setup):**

1. **PostgreSQL Server (External):**
   ```sql
   -- Create database and user (one-time setup)
   CREATE DATABASE mqtt2bdd;
   CREATE USER mqtt2bdd WITH PASSWORD 'strong_password';
   GRANT ALL PRIVILEGES ON DATABASE mqtt2bdd TO mqtt2bdd;

   -- Connect to database
   \c mqtt2bdd

   -- Execute schema from dev/init-db/01-schema.sql
   CREATE TABLE sensor_metrics (
       id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
       sensor VARCHAR(255) NOT NULL,
       date TIMESTAMP NOT NULL,
       metrics JSONB NOT NULL,
       CONSTRAINT unique_sensor_date UNIQUE (sensor, date)
   );

   CREATE INDEX idx_sensor_metrics_sensor_date ON sensor_metrics (sensor, date DESC);
   CREATE INDEX idx_sensor_metrics_metrics ON sensor_metrics USING GIN (metrics);
   ```

2. **Mosquitto Broker (External):**
   - Already running and accessible
   - Configured with anonymous access or authentication
   - Accessible from Docker container network

3. **Network Configuration:**
   - MQTT2BDD container must reach PostgreSQL and Mosquitto
   - Verify firewall rules allow connections
   - Test connectivity: `docker run --rm postgres:15-alpine psql -h postgres.local -U mqtt2bdd -d mqtt2bdd`

**Simplified Deployment:**
```bash
# On Proxmox host
cd /opt/mqtt2bdd

# Configure environment
cp .env.prod.example .env.prod
nano .env.prod  # Edit POSTGRES_PASSWORD and other settings

# Deploy application only
docker-compose -f docker-compose.prod.yml up -d

# Verify logs
docker logs mqtt2bdd-prod-app --tail 50

# Expected logs:
# INFO: "MQTT2BDD starting" version=0.1.0
# INFO: "MQTT connected" broker=mosquitto.local:1883
# INFO: "Database connected" host=postgres.local
# INFO: "Subscribed to all topics"
```

**Rationale for External Services:**
- PostgreSQL and Mosquitto are shared infrastructure (used by other services)
- Centralized management and backup
- No need to duplicate database/broker in MQTT2BDD deployment
- Simpler docker-compose (single application container)

### Environment Promotion Flow

```
┌─────────────────────┐
│   Developer Local   │
│   (dev/ compose)    │
└──────────┬──────────┘
           │ git push
           ▼
┌─────────────────────┐
│   GitHub Actions    │
│   CI/CD Pipeline    │
│   (build + test)    │
└──────────┬──────────┘
           │ on success
           ▼
┌─────────────────────┐
│  Integration Tests  │
│  (test/ compose)    │
└──────────┬──────────┘
           │ on success + git tag
           ▼
┌─────────────────────┐
│  GitHub Release     │
│  Docker Image Build │
│  (optional: push)   │
└──────────┬──────────┘
           │ manual deploy
           ▼
┌─────────────────────┐
│  Proxmox Production │
│  (compose.prod.yml) │
│  External PG/MQTT   │
└─────────────────────┘
```

**Promotion Steps:**

1. **Local Development → GitHub:**
   - Developer commits code
   - `git push origin feature-branch`
   - Opens Pull Request

2. **GitHub → CI Validation:**
   - GitHub Actions runs CI pipeline (format, vet, staticcheck, tests, build)
   - PR approved and merged to `main`

3. **Main → Integration Tests:**
   - Merge triggers CI on `main` branch
   - Integration test suite runs in isolated environment
   - All tests must pass

4. **Integration → Release:**
   - Developer creates git tag: `git tag v0.1.0 && git push origin v0.1.0`
   - Release pipeline triggers
   - Docker image built with version
   - Optional: Push to Docker Hub

5. **Release → Production:**
   - **Manual deployment** to Proxmox (intentional gate)
   - SSH to Proxmox server
   - Pull code/image
   - Run `docker-compose -f docker-compose.prod.yml up -d`
   - Verify deployment via logs and health checks

**Promotion Criteria:**
- ✅ All unit tests pass
- ✅ All integration tests pass
- ✅ Code passes staticcheck and go vet
- ✅ Docker image builds successfully
- ✅ Git tag created (semantic versioning: vX.Y.Z)
- ✅ Manual approval for production deployment

### Rollback Strategy

**Primary Method:** Docker Compose with previous image version

**Trigger Conditions:**
- Application fails to start (health check fails)
- Critical bugs discovered (message loss, database corruption)
- Performance degradation (buffer overflow, CPU/memory spikes)
- Database connection failures not recovering

**Recovery Time Objective (RTO):** < 5 minutes

**Rollback Procedure:**

**1. Immediate Rollback (Docker Compose):**
```bash
# SSH to Proxmox server
ssh user@proxmox-server
cd /opt/mqtt2bdd

# Stop current version
docker-compose -f docker-compose.prod.yml down

# Checkout previous version tag
git checkout v0.0.9  # Previous stable version

# Rebuild from previous version
docker build --build-arg VERSION=0.0.9 -t mqtt2bdd:0.0.9 .

# OR: Update .env.prod to use previous image version
echo "VERSION=0.0.9" > .env.prod

# Start previous version
docker-compose -f docker-compose.prod.yml up -d

# Verify rollback
docker-compose -f docker-compose.prod.yml logs -f mqtt2bdd
```

**2. Database Rollback (if schema changed):**
```bash
# If new version included schema changes (rare for this project)
# Stop application
docker-compose -f docker-compose.prod.yml down

# Restore database backup on external PostgreSQL server
psql -h postgres.local -U postgres mqtt2bdd < backup_20260213.sql

# Start previous application version
docker-compose -f docker-compose.prod.yml up -d
```

**Rollback Verification Checklist:**
- [ ] Application logs show "MQTT2BDD starting" with correct version
- [ ] MQTT connection established (log: "MQTT connected")
- [ ] Database connection established (log: "Database connected")
- [ ] Subscribed to MQTT topics (log: "Subscribed to all topics")
- [ ] Test message flows through system
- [ ] Database query confirms message persisted
- [ ] Health check passes (60 second observation)

**Rollback Decision Matrix:**

| Severity | Condition | Action | Timeline |
|----------|-----------|--------|----------|
| **Critical** | Data loss detected | Immediate rollback + restore backup | < 5 min |
| **High** | App crash loop | Immediate rollback | < 3 min |
| **Medium** | Performance degradation | Monitor 15 min → rollback if persists | < 20 min |
| **Low** | Non-critical bug | Fix forward in next release | N/A |

**Post-Rollback Actions:**
1. Document incident in `.ai/incidents.md`
2. Create GitHub issue for bug investigation
3. Add regression test
4. Fix bug on feature branch
5. Re-test before next release

### Backup Strategy

**Database Backups (External PostgreSQL):**

Since PostgreSQL is managed externally, backups are handled at the PostgreSQL server level (not in MQTT2BDD scope). Coordinate with Proxmox administrator for:

- Daily automated backups
- Retention policy (30 days daily, 3 months weekly, 1 year monthly)
- Backup verification
- Restore procedures

**Application State:**
- No persistent application state (stateless)
- Configuration in `.env.prod` (version-controlled)
- Docker images versioned and tagged

### Monitoring and Observability

**Log Aggregation:**
- Docker JSON logs with rotation (max-size: 10m, max-file: 3)
- Centralized log viewing: `docker logs mqtt2bdd-prod-app -f`
- Optional: Forward to Grafana Loki or ELK (future enhancement)

**Health Monitoring:**
- Application health logs (every 60s): "Health check: MQTT=connected, DB=connected, Buffer=N/1000"
- Docker container status: `docker-compose ps`

**Metrics (Future Enhancement):**
- Prometheus exporter for Go application
- Grafana dashboards for real-time monitoring

**Alerting (Future Enhancement):**
- Email/Slack on container failures
- Alert on buffer >80% full

---

## Error Handling Strategy

_[To be completed]_

---

## Coding Standards

_[To be completed]_

---

## Test Strategy and Standards

_[To be completed]_

---

## Security

_[To be completed]_

---

## Checklist Results Report

_[To be completed before final review]_

---

## Next Steps

After completing the architecture:

1. Continue architecture sections (Infrastructure, Error Handling, Coding Standards, Testing, Security)
2. Review with Product Owner
3. Begin story implementation with Dev agent
4. Set up infrastructure with DevOps processes
