# Project Brief: MQTT2BDD

## Executive Summary

**MQTT2BDD** is a learning-oriented Go application that bridges MQTT home automation notifications to PostgreSQL for persistent storage and future analysis. The application subscribes to all MQTT topics from a Zigbee2MQTT home automation setup (thermostats, sensors, switches) and stores each notification with its full JSON payload in a PostgreSQL database, enabling time-series analysis and visualization through Grafana.

**Primary Problem:** Home automation data from MQTT is ephemeral - without persistence, historical analysis of temperature trends, energy consumption patterns, and device behavior is impossible.

**Target User:** Solo developer/home automation enthusiast seeking to learn Go while building practical infrastructure for their smart home.

**Key Value Proposition:** Production-ready, educational Go codebase demonstrating best practices for MQTT client handling, PostgreSQL integration, resilient connection management, and structured logging - all while solving a real-world home automation need.

---

## Problem Statement

**Current State:**
Home automation systems like Zigbee2MQTT publish device state changes and sensor readings via MQTT in real-time. These notifications are ephemeral - they exist only in the moment they're published. Once consumed or missed, the data is lost forever.

**Pain Points:**
- **No Historical Context:** Cannot answer questions like "What was the living room temperature last Tuesday at 3pm?" or "How often did the thermostat cycle yesterday?"
- **No Trend Analysis:** Impossible to identify patterns (temperature fluctuations, energy consumption trends, device reliability issues)
- **No Data-Driven Decisions:** Cannot optimize heating schedules or identify malfunctioning sensors without historical data
- **Grafana Integration Blocked:** Cannot leverage powerful visualization tools without a persistent data store

**Impact:**
With ~8 thermostats currently and up to ~100 connected devices planned (switches, gas/electricity meters, sensors), valuable operational data is continuously lost. This prevents:
- Energy optimization opportunities
- Predictive maintenance of devices
- Debugging intermittent issues that require historical context
- Learning from usage patterns

**Why Existing Solutions Fall Short:**
- Generic MQTT-to-database bridges often require complex configuration per topic
- Commercial solutions are overkill and expensive for home use
- No solution addresses the **learning objective** of understanding Go while building production infrastructure

**Urgency:**
Every day without this system means lost data that could inform home automation decisions. Additionally, the learning value is time-sensitive - building this project now establishes Go fundamentals that enable future smart home enhancements.

---

## Proposed Solution

**Core Concept:**
MQTT2BDD is a standalone Go application that acts as a persistent MQTT subscriber, capturing all messages from the home automation broker and storing them in a structured PostgreSQL database. The application runs autonomously in a Docker container, providing a resilient bridge between the ephemeral MQTT message bus and persistent storage.

**Key Components:**
1. **MQTT Listener** - Subscribes to all topics (`#` wildcard) and receives JSON payloads from Zigbee2MQTT devices
2. **Data Normalizer** - Extracts topic (sensor identifier), timestamp, and JSON payload from each message
3. **PostgreSQL Writer** - Inserts normalized data into the `sensor_metrics` table with conflict handling
4. **Resilience Layer** - Automatic reconnection logic for both MQTT broker and database failures
5. **Structured Logging** - Normal and debug modes for operational visibility

**Key Differentiators:**
- **Zero Configuration Per Device:** Works with any MQTT topic/payload structure automatically
- **Educational Architecture:** Code organized to teach Go best practices (packages, interfaces, error handling, concurrency)
- **Production-Ready Patterns:** Connection pooling, graceful shutdown, health checks, structured logging
- **Docker-Native:** Designed for containerized deployment on Alpine Linux 3.23 with environment-based configuration
- **Lightweight Deployment:** Optimized for minimal container footprint on Alpine base image

**Why This Solution Will Succeed:**
- **Simplicity:** No complex routing rules or transformations - just capture everything
- **Flexibility:** JSON storage allows querying arbitrary sensor fields without schema migrations
- **Scalability:** Can handle current 8 devices and scale to 100+ without architectural changes
- **Maintainability:** Well-documented, idiomatic Go code enables future enhancements

**High-Level Vision:**
A reference implementation demonstrating how to build robust, production-grade Go services that integrate message brokers with databases - knowledge transferable to professional software engineering contexts far beyond home automation.

---

## Target Users

### Primary User Segment: Solo Developer / Home Automation Enthusiast

**Profile:**
- **Technical Level:** Software developer or technically-savvy enthusiast with programming experience (but not necessarily in Go)
- **Context:** Owns a home automation setup (Zigbee2MQTT + Mosquitto MQTT broker)
- **Motivation:** Dual purpose - learn Go language fundamentals while solving a real infrastructure need

**Current Behaviors & Workflows:**
- Manages home automation devices through Zigbee2MQTT
- Uses tools like `mosquitto_sub` to manually inspect MQTT messages
- Wants to visualize home metrics in Grafana but lacks data persistence layer
- Comfortable with Docker, command-line tools, and basic database operations

**Specific Needs & Pain Points:**
- Cannot analyze historical home automation data (temperature trends, energy patterns)
- Existing MQTT bridge solutions are either too complex or don't serve learning objectives
- Needs well-documented, educational code to understand Go patterns and idioms
- Requires production-quality error handling and resilience patterns to learn proper practices

**Goals:**
- **Learning Goal:** Understand Go fundamentals (concurrency, interfaces, error handling, package structure)
- **Functional Goal:** Capture all MQTT home automation data for future Grafana visualization
- **Operational Goal:** Run a reliable, autonomous service that requires minimal maintenance
- **Knowledge Transfer Goal:** Build skills applicable to professional Go development

---

## Goals & Success Metrics

### Business Objectives

- **Go Language Mastery:** Complete a production-grade Go project demonstrating idiomatic patterns, achieving comfort with goroutines, channels, interfaces, and error handling within 4-6 weeks
- **Data Capture Reliability:** Achieve 99%+ capture rate of all MQTT messages published by home automation devices with automatic recovery from failures
- **Operational Autonomy:** Deploy a zero-maintenance service that runs continuously in Docker without manual intervention for connection failures or restarts
- **Knowledge Foundation:** Build transferable skills enabling professional Go development work or future smart home projects

### User Success Metrics

- **Code Comprehensibility:** A Go beginner can read the codebase and understand the architecture, data flow, and key patterns within 1-2 hours
- **Learning Velocity:** Each major component (MQTT client, DB integration, logging, error handling) teaches a distinct Go concept with clear documentation
- **Deployment Simplicity:** Application can be deployed via Docker Compose with environment variables only - no code changes required
- **Debug Visibility:** Logs provide clear insight into application state, making troubleshooting straightforward

### Key Performance Indicators (KPIs)

- **Uptime:** 99.5%+ service availability over 30-day periods
- **Message Capture Rate:** 100% of published MQTT messages successfully stored in PostgreSQL (measured via topic counting)
- **Recovery Time:** Automatic reconnection to MQTT/DB within 30 seconds of connection loss
- **Storage Efficiency:** PostgreSQL database size remains manageable (<1GB/month for ~100 devices)
- **Log Clarity:** Zero ambiguous error messages - all failures have clear, actionable log entries
- **Code Quality:** All code passes `go fmt`, `go vet`, and follows effective Go conventions
- **Test Coverage:** Comprehensive unit tests for all core components (MQTT handler, database operations, data normalization) with >80% code coverage

---

## MVP Scope

### Core Features (Must Have)

- **MQTT Subscription Handler:** Subscribe to all topics (`#` wildcard) on configured MQTT broker, receive and parse JSON messages
- **PostgreSQL Integration:** Connect to PostgreSQL database and insert normalized records into `sensor_metrics` table (sensor VARCHAR, date TIMESTAMP, metrics JSON)
- **Schema Validation at Startup:** Verify `sensor_metrics` table exists with expected schema (sensor VARCHAR, date TIMESTAMP, metrics JSON) on application start, failing fast with clear error if schema is invalid or missing
- **Automatic Reconnection:** Detect and recover from MQTT broker or database disconnections within 30 seconds with exponential backoff
- **Environment-Based Configuration:** All settings (MQTT host/port/credentials, PostgreSQL connection string) configurable via environment variables
- **Structured Logging:** Implement log levels (INFO, DEBUG, ERROR) with clear, actionable messages for startup, shutdown, connections, and errors
- **Graceful Shutdown:** Handle SIGTERM/SIGINT signals cleanly, closing connections and flushing pending writes before exit
- **Docker Deployment:** Dockerfile based on Alpine Linux 3.23 producing a minimal container image with single-binary deployment
- **Comprehensive Documentation:** README with architecture overview, setup instructions, configuration reference, and code walkthrough for Go learners
- **Unit Test Suite:** Test coverage >80% for all core components with examples demonstrating Go testing best practices

### Out of Scope for MVP

- Topic filtering / whitelist configuration (future enhancement)
- Data transformation or enrichment before storage
- Web UI or API for querying stored data
- Grafana dashboards or integration (separate project phase)
- Database schema migration tools
- Metrics/monitoring endpoints (Prometheus, health checks)
- Data retention/purge policies
- Multiple MQTT broker support
- Message deduplication logic

### MVP Success Criteria

The MVP is considered successful when:
1. Application runs continuously for 7 days without manual intervention
2. All MQTT messages from 8 thermostats are captured in PostgreSQL with correct timestamps
3. Application automatically recovers from simulated MQTT/DB outages within 30 seconds
4. A Go beginner can read the README, understand the architecture, and successfully deploy via Docker Compose
5. Unit tests pass with >80% coverage and demonstrate clear testing patterns
6. Logs provide sufficient information to debug any operational issues
7. Application fails immediately at startup with clear error message if database schema is incorrect or missing

---

## Post-MVP Vision

### Phase 2 Features (Optional Enhancements)

If operational needs evolve, potential simple additions:

- **Topic Filtering:** Configuration option to whitelist/blacklist specific topics if a device becomes too noisy
- **Data Retention Script:** Simple SQL script or cron job to purge old data if database growth becomes an issue
- **Basic Health Check:** Minimal HTTP endpoint for container health monitoring if needed

### Long-term Vision

The core philosophy is to **keep MQTT2BDD simple and focused**. The application should remain a straightforward, reliable MQTT→PostgreSQL bridge. Future work is more likely to involve:

- Building Grafana dashboards on top of the captured data
- Separate projects for specific home automation needs
- Applying learned Go patterns to other projects

### Design Principle

**"Do one thing well"** - MQTT2BDD captures all MQTT data to PostgreSQL reliably. Complex analytics, transformations, and visualizations belong in separate tools (Grafana, custom scripts, etc.) that consume this data.

---

## Technical Considerations

### Platform Requirements

- **Target Platform:** Docker container on Alpine Linux 3.23
- **Runtime:** Go 1.21+ compiled binary (statically linked for Alpine compatibility)
- **Deployment:** Single-container deployment via Docker Compose
- **Performance Requirements:** Handle ~100 MQTT messages/minute with <100ms processing latency per message

### Technology Stack

- **Language:** Go (Golang) - chosen for learning objectives and strong concurrency primitives
- **MQTT Client Library:** `github.com/eclipse/paho.mqtt.golang` (official Eclipse Paho client, widely used and well-documented)
- **PostgreSQL Driver:** `github.com/jackc/pgx/v5` (modern, high-performance driver with excellent error handling, industry standard)
- **Logging:** Standard library `log/slog` (Go 1.21+) for structured logging
- **Configuration:** Standard library `os.Getenv()` for environment variable handling (zero dependencies, clear and educational)
- **Testing:** Standard library `testing` package with table-driven tests

### Architecture Considerations

**Repository Structure:**
- Single repository (monorepo not needed)
- Clean package layout: `cmd/`, `internal/`, `pkg/` following Go project structure best practices
- `cmd/mqtt2bdd/main.go` as entry point
- `dev/` directory: Docker Compose stack for development (PostgreSQL, Mosquitto, init scripts)
- `test/` directory: Docker Compose stack for isolated integration testing
- No local installation of PostgreSQL or Mosquitto required - fully containerized development

**Service Architecture:**
- Single-process application (no microservices complexity)
- Goroutine-based concurrency for MQTT message handling
- Connection pooling for PostgreSQL via `database/sql` + `pgx`

**Integration Requirements:**
- MQTT broker connection (Mosquitto) - standard TCP connection
- PostgreSQL database - standard connection string authentication
- No external dependencies beyond MQTT + PostgreSQL

**Security/Compliance:**
- MQTT authentication via username/password (if broker requires)
- PostgreSQL SSL/TLS support optional (depends on deployment environment)
- No PII storage - just device IDs and sensor readings
- Secrets managed via environment variables (Docker secrets or .env file)

**Containerized Development Strategy:**

The project adopts a **fully containerized development approach** to eliminate "works on my machine" issues and streamline onboarding:

**Directory Structure:**
```
mqtt2bdd/
├── cmd/                    # Application entry points
├── internal/               # Private application code
├── pkg/                    # Public libraries (if any)
├── dev/                    # Development environment
│   ├── docker-compose.yml  # PostgreSQL + Mosquitto stack
│   ├── init-db/            # SQL scripts for schema initialization
│   └── .env.example        # Example configuration
├── test/                   # Test environment
│   ├── docker-compose.yml  # Isolated test stack
│   └── init-db/            # Test database initialization
└── Dockerfile              # Production application image
```

**Development Workflow:**
1. Developer clones repository
2. Runs `cd dev && docker-compose up` to start PostgreSQL + Mosquitto
3. Develops application code (runs locally or in container)
4. Application connects to containerized infrastructure via `localhost:5432` (PostgreSQL) and `localhost:1883` (MQTT)

**Test Workflow:**
1. Integration tests run against `test/docker-compose.yml` stack
2. Isolated environment prevents test data from polluting development database
3. Can tear down and rebuild test environment cleanly between test runs

**Benefits:**
- **Zero Local Installation:** No PostgreSQL or Mosquitto packages installed on host OS
- **Consistent Environments:** All developers use identical infrastructure versions
- **Fast Onboarding:** New developer productive in minutes (`git clone` + `docker-compose up`)
- **Clean Separation:** Dev and test environments are completely isolated
- **Easy Teardown:** `docker-compose down -v` removes all infrastructure cleanly

---

## Constraints & Assumptions

### Constraints

- **Budget:** $0 - using only free, open-source tools and libraries
- **Timeline:** MVP target of 4-6 weeks (learning Go + building functional application)
- **Resources:** Solo developer project with limited time (evenings/weekends)
- **Technical:**
  - Must run on Alpine Linux 3.23 Docker container
  - Must work with existing PostgreSQL `sensor_metrics` table schema
  - Must integrate with existing Zigbee2MQTT + Mosquitto setup without modifying those systems

### Key Assumptions

- PostgreSQL database and MQTT broker are on the same local network (low latency, high reliability)
- MQTT messages are valid JSON (no binary payloads or malformed data handling required for MVP)
- Database has sufficient storage capacity (~1GB/month estimated growth)
- Network between services is trusted (home network, not public internet)
- MQTT broker and PostgreSQL are maintained separately (this application doesn't manage their lifecycle)
- Single instance deployment is sufficient (no clustering or high-availability requirements)
- Message ordering is not critical (occasional out-of-order messages acceptable)

---

## Risks & Mitigation Strategies

### Technical Risks

**Risk 1: MQTT Connection Stability**
- **Severity:** High
- **Description:** If MQTT broker restarts or network hiccups occur, messages could be lost
- **Mitigation:**
  - Implement automatic reconnection with exponential backoff (5s, 10s, 30s, 60s intervals)
  - Use persistent MQTT sessions with QoS 1 (at-least-once delivery guarantee)
  - Log all connection state changes for troubleshooting
  - Test reconnection logic with simulated outages

**Risk 2: PostgreSQL Write Performance Bottleneck**
- **Severity:** Medium
- **Description:** High MQTT message volume could overwhelm database writes, causing backpressure
- **Mitigation:**
  - Implement buffered channel (capacity: 1000 messages) between MQTT handler and DB writer
  - Use batch inserts (e.g., 100 messages per transaction) if message rate exceeds threshold
  - Monitor channel depth and log warnings if buffer fills beyond 80%
  - Establish max message rate baseline during testing (target: handle 200 msg/min comfortably)

**Risk 3: JSON Schema Variability**
- **Severity:** Low
- **Description:** Different devices may publish incompatible JSON structures or malformed payloads
- **Mitigation:**
  - Store raw JSON as JSONB without validation (PostgreSQL handles malformed JSON gracefully)
  - Log warning for non-JSON payloads but don't crash application
  - Document any problematic devices discovered during operation
  - Future enhancement: optional JSON schema validation per topic pattern

**Risk 4: Database Schema Mismatch**
- **Severity:** High
- **Description:** Application expects specific schema; if PostgreSQL table schema is wrong or missing, data writes will fail
- **Mitigation:**
  - Implement schema validation check on startup that verifies `sensor_metrics` table structure
  - Fail fast with clear error message if schema is invalid or table doesn't exist
  - Document exact SQL CREATE TABLE statement in README for user reference
  - Consider future enhancement: auto-create table if missing (post-MVP)

**Risk 5: Go Learning Curve**
- **Severity:** Medium
- **Description:** As primary developer is learning Go, architectural mistakes could require refactoring
- **Mitigation:**
  - Follow established Go project layout conventions (`golang-standards/project-layout`)
  - Review Go best practices documentation before implementing each component
  - Start with simple, working implementation then refactor iteratively
  - Accept that "good enough" is acceptable for learning project (avoid perfectionism paralysis)
  - Leverage AI assistance (Claude, GitHub Copilot) for idiomatic pattern guidance

### Operational Risks

**Risk 6: Silent Failure / Missing Logs**
- **Severity:** Medium
- **Description:** Application could stop working without clear indication of what failed
- **Mitigation:**
  - Implement structured logging with clear levels (INFO for normal ops, ERROR for failures)
  - Log critical lifecycle events: startup, shutdown, connection established/lost, write errors
  - Use DEBUG mode toggle for verbose logging during troubleshooting
  - Include context in all error messages (what failed, why, suggested action)

**Risk 7: Data Loss During Shutdown**
- **Severity:** Low
- **Description:** In-flight messages could be lost if container is stopped abruptly
- **Mitigation:**
  - Implement graceful shutdown handler (SIGTERM/SIGINT)
  - Flush buffered messages to database before exit
  - Set reasonable shutdown timeout (30 seconds) in Docker Compose
  - Document that aggressive container kill (`docker kill -9`) may lose last few seconds of data

**Risk 8: Database Storage Exhaustion**
- **Severity:** Low (long-term concern)
- **Description:** Database could grow unbounded over months/years
- **Mitigation:**
  - Monitor database size weekly during first month
  - Document expected growth rate (~1GB/month for 100 devices)
  - Plan future data retention script (e.g., purge data older than 6 months)
  - Not a critical MVP concern given projected growth rate

### Scope Risks

**Risk 9: Feature Creep**
- **Severity:** Medium
- **Description:** Temptation to add "just one more feature" could delay MVP or complicate codebase
- **Mitigation:**
  - Strictly enforce MVP scope defined in this document
  - Maintain "Future Enhancements" backlog for good ideas
  - Complete MVP success criteria before considering any additions
  - Remember: primary goal is learning Go, not building perfect tool

**Risk 10: Development Environment Setup Complexity**
- **Severity:** Very Low (mitigated by design)
- **Description:** Traditionally, setting up PostgreSQL + MQTT broker locally can be complex and error-prone
- **Mitigation:**
  - **Fully containerized development environment** via `dev/docker-compose.yml`
  - Single command setup: `cd dev && docker-compose up`
  - No local installation of PostgreSQL or Mosquitto required
  - Consistent environment across all developers (eliminates "works on my machine")
  - Clean teardown with `docker-compose down -v`
  - Separate `test/docker-compose.yml` for isolated integration testing

---

## Development Roadmap

### Phase 0: Project Setup (Week 1)
**Duration:** 3-5 days
**Goal:** Establish development environment and project structure

**Key Activities:**
- Initialize Go module and repository structure (`cmd/`, `internal/`, `pkg/`)
- Create `dev/` directory with Docker Compose for development infrastructure (PostgreSQL + Mosquitto)
- Create `test/` directory with Docker Compose for test infrastructure (isolated instances)
- Write SQL initialization script for `sensor_metrics` table (auto-executed on PostgreSQL container startup)
- Configure development Docker Compose with volume mounts for live code reloading
- Write initial README with one-command development setup (`docker-compose up` in `dev/`)
- Configure `.gitignore` and basic project files

**Deliverables:**
- Buildable Go project skeleton
- `dev/docker-compose.yml` - Complete development stack (PostgreSQL, Mosquitto, optional: application container)
- `test/docker-compose.yml` - Isolated test stack for integration tests
- SQL initialization scripts in `dev/init-db/` and `test/init-db/`
- README with "Getting Started" - new developers can start with just Docker installed

---

### Phase 1: Core MQTT Integration (Week 2)
**Duration:** 5-7 days
**Goal:** Establish reliable MQTT subscription and message handling

**Key Activities:**
- Implement MQTT client connection using `paho.mqtt.golang`
- Subscribe to wildcard topic (`#`) with QoS 1
- Parse incoming MQTT messages (extract topic, timestamp, JSON payload)
- Implement structured logging for MQTT events
- Write unit tests for message parsing logic

**Deliverables:**
- Working MQTT subscriber that logs all received messages
- Unit tests covering message parsing and error cases
- Debug mode showing real-time MQTT message flow

**Success Criteria:**
- Application successfully connects to MQTT broker
- All messages from 8 thermostats are received and logged
- Logs clearly show topic, timestamp, and payload for each message

---

### Phase 2: PostgreSQL Integration (Week 3)
**Duration:** 5-7 days
**Goal:** Persist MQTT messages to PostgreSQL database

**Key Activities:**
- Implement PostgreSQL connection using `pgx/v5` driver
- Create database writer goroutine with buffered channel
- Implement INSERT logic for `sensor_metrics` table
- Add schema validation on application startup
- Handle database connection errors with retry logic
- Write unit tests for database operations

**Deliverables:**
- MQTT messages successfully written to PostgreSQL
- Schema validation failing fast on startup if table schema incorrect
- Unit tests for database writer with mock/test database

**Success Criteria:**
- All received MQTT messages appear in PostgreSQL within 5 seconds
- Application validates database schema on startup and fails with clear error if invalid
- Database writer handles connection errors gracefully

---

### Phase 3: Resilience & Error Handling (Week 4)
**Duration:** 5-7 days
**Goal:** Implement production-ready reliability patterns

**Key Activities:**
- Implement automatic MQTT reconnection with exponential backoff
- Implement automatic PostgreSQL reconnection logic
- Add graceful shutdown handler (SIGTERM/SIGINT)
- Enhance structured logging with error context
- Implement message buffering to handle temporary DB outages
- Simulate failures (kill MQTT broker, kill DB) and verify recovery

**Deliverables:**
- Application automatically recovers from MQTT/DB outages
- Graceful shutdown flushes buffered messages
- Clear, actionable error messages for all failure scenarios

**Success Criteria:**
- Application reconnects to MQTT broker within 30 seconds of disconnect
- Application reconnects to PostgreSQL within 30 seconds of disconnect
- No message loss during temporary outages (up to 1000 message buffer capacity)
- Clean shutdown on SIGTERM with all pending messages written

---

### Phase 4: Docker Deployment (Week 5)
**Duration:** 3-5 days
**Goal:** Containerize application for production deployment

**Key Activities:**
- Create multi-stage Dockerfile (builder + Alpine runtime)
- Configure environment variable handling
- Write Docker Compose configuration for full stack
- Test statically-linked binary on Alpine Linux 3.23
- Document deployment process in README

**Deliverables:**
- Minimal Docker image (<20MB) based on Alpine
- Docker Compose file for single-command deployment
- Deployment documentation with environment variable reference

**Success Criteria:**
- Application runs successfully in Docker container
- Configuration entirely via environment variables
- Container starts, connects, and persists data without code changes

---

### Phase 5: Testing & Documentation (Week 6)
**Duration:** 5-7 days
**Goal:** Achieve production readiness with comprehensive testing and documentation

**Key Activities:**
- Expand unit test coverage to >80%
- Write integration tests (MQTT → PostgreSQL end-to-end)
- Conduct 7-day continuous operation test
- Write comprehensive README (architecture, setup, troubleshooting)
- Document code with inline comments explaining Go patterns for learners
- Create troubleshooting guide for common issues

**Deliverables:**
- Test suite with >80% code coverage
- Complete README with architecture diagram
- Troubleshooting guide
- Code comments highlighting educational Go patterns

**Success Criteria:**
- All unit tests pass (`go test ./...`)
- Application runs for 7 consecutive days without manual intervention
- A Go beginner can read README and understand architecture within 1-2 hours
- All MVP success criteria are met

---

### MVP Completion Checkpoint
**Target:** End of Week 6
**Review Criteria:**
1. ✅ Application runs continuously for 7 days without manual intervention
2. ✅ All MQTT messages from 8 thermostats captured in PostgreSQL
3. ✅ Automatic recovery from simulated MQTT/DB outages within 30 seconds
4. ✅ A Go beginner can deploy via Docker Compose following README
5. ✅ Unit tests pass with >80% coverage
6. ✅ Logs provide sufficient debug information
7. ✅ Application fails immediately at startup with clear error if database schema invalid

**If criteria met:** MVP is complete → move to operational deployment
**If criteria not met:** Identify gaps and iterate on failing areas

---

## Dependencies & Critical Success Factors

### External Dependencies

**Production Infrastructure Dependencies:**
- **PostgreSQL Database (v12+):** Must be running and accessible with `sensor_metrics` table created
- **MQTT Broker (Mosquitto):** Must be running and publishing Zigbee2MQTT messages
- **Docker Engine (v20.10+):** Required for containerized deployment
- **Network Connectivity:** Stable local network between application, MQTT broker, and PostgreSQL

**Development Dependencies:**
- **Docker & Docker Compose:** Only requirement for development - no local PostgreSQL or Mosquitto installation needed
- **Go 1.21+:** For building and running the application (can also run in container)
- **Go Modules:** `paho.mqtt.golang` and `pgx/v5` must be available via public repositories

**Development Environment Setup:**
- `dev/docker-compose.yml` provides complete development stack (PostgreSQL + Mosquitto)
- `test/docker-compose.yml` provides isolated test environment
- New developers need only: Git, Docker, and Go installed (or Go in container)
- One command to start: `cd dev && docker-compose up`

**Mitigation for External Dependencies:**
- All dependencies are open-source and free (no vendor lock-in risk)
- PostgreSQL and Mosquitto are mature, stable projects with strong community support
- Application is designed to tolerate temporary outages of MQTT/DB (automatic reconnection)

### Critical Success Factors

**CSF 1: Learning-Optimized Code Structure**
- Code must balance production quality with educational clarity
- Clear package separation demonstrating Go project organization
- Inline comments explaining "why" behind architectural decisions
- README must serve as learning guide, not just operational manual

**CSF 2: Operational Reliability**
- Application must "just work" after deployment without constant maintenance
- Automatic recovery from failures is non-negotiable for success
- Logs must provide actionable information for troubleshooting

**CSF 3: Simplicity Over Perfection**
- Avoid premature optimization or over-engineering
- Accept "good enough" solutions that satisfy MVP criteria
- Resist feature creep - defer enhancements to post-MVP phase

**CSF 4: Time Management**
- Strict adherence to 4-6 week timeline prevents project abandonment
- Each phase has clear deliverables and success criteria
- Weekly checkpoints to assess progress and adjust scope if needed

**CSF 5: Schema Validation**
- Application must verify database schema on startup
- Fast failure with clear error message prevents silent data loss
- User must have confidence that database structure is correct before any data is written

**CSF 6: Frictionless Development Setup**
- Containerized development environment eliminates setup friction
- New developers productive within minutes (not hours/days)
- Consistent infrastructure across all development machines
- Clean separation between dev and test environments
- Zero pollution of host OS with database/broker installations

### Blockers & Escalation

**Potential Blockers:**
- Go learning curve steeper than anticipated → Allocate extra time for Phase 1-2, leverage learning resources
- PostgreSQL performance issues with high message volume → Implement batch inserts earlier than planned
- Docker/Alpine compatibility issues → Test Alpine deployment earlier in Phase 1-2
- Database schema issues → Implement schema validation in Phase 2 to detect early

**De-Risking Strategy:**
- Start each phase with smallest possible working implementation (spike/prototype)
- Test high-risk components (MQTT reconnection, DB resilience) early with dedicated experiments
- If blocked for >2 days on any issue, scope down that feature to unblock progress

---

## Conclusion

**MQTT2BDD** represents a pragmatic fusion of learning and utility - a production-quality Go application solving a real-world home automation need while teaching foundational language skills. By capturing ephemeral MQTT messages into persistent PostgreSQL storage, the application unlocks historical analysis, trend visualization, and data-driven home automation decisions.

### Why This Project Matters

**For Learning:**
- Hands-on experience with Go's concurrency primitives (goroutines, channels)
- Real-world application of interfaces, error handling, and package design
- Exposure to production patterns: connection pooling, graceful shutdown, structured logging
- Transferable skills applicable to professional software engineering

**For Home Automation:**
- Foundation for Grafana dashboards visualizing temperature, energy, and device behavior
- Historical data enabling optimization of heating schedules and energy consumption
- Debug capability for intermittent device issues requiring time-series context
- Scalable architecture supporting growth from 8 to 100+ devices

### Next Steps

1. **Immediate:** Review this brief with stakeholders (yourself!) and confirm alignment on scope, timeline, and success criteria
2. **Week 1:** Begin Phase 0 (Project Setup) following the roadmap outlined above
3. **Weekly:** Checkpoint against phase deliverables and adjust scope if needed
4. **Week 6:** Evaluate MVP completion criteria and decide on production deployment

### Closing Thought

The true measure of success is not feature completeness, but rather **learning achieved** and **operational reliability delivered**. A simple, well-understood application that runs autonomously for months is far more valuable than a feature-rich codebase that's abandoned due to complexity.

**Let's build something simple, robust, and educational.** 🚀

