# MQTT2BDD

![Go Version](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go&logoColor=white)

## Overview

MQTT2BDD is a Go application that bridges home automation MQTT messages into a PostgreSQL
database for persistent storage and historical analysis. It subscribes to **all** topics on a
configured broker using a single wildcard subscription (`#`) — no per-sensor configuration is
ever needed — and inserts each message's topic, timestamp, and JSON payload into a
`sensor_metrics` table. The stored data is intended for downstream analysis and visualization
tools such as Grafana; this project is only concerned with reliably getting MQTT data into
PostgreSQL, not with visualizing it.

Beyond its practical purpose, this codebase is written to be read: it demonstrates idiomatic
Go concurrency (goroutines and channels), structured logging, twelve-factor configuration, and
graceful shutdown — patterns worth studying whether or not you ever deploy it.

## Features

- Wildcard MQTT subscription (`#`) — device-agnostic, no per-sensor configuration needed.
- Automatic MQTT reconnection with resubscribe retry, so a broker restart or network blip does
  not require a manual restart of the application.
- In-memory buffered channel decoupling MQTT reception from database writes, with backpressure:
  the producer blocks once the buffer (`BUFFER_SIZE`, default 1000) is full, rather than
  dropping messages silently.
- Automatic insert retry on database outage — a message is retried every 10 seconds until it
  succeeds, so no message is lost during an outage.
- Idempotent writes via `INSERT ... ON CONFLICT (sensor, date) DO NOTHING`.
- Graceful shutdown on SIGTERM/SIGINT: stops the MQTT client, drains all buffered messages
  (bounded by a 30-second timeout), then closes the database pool in order.
- Periodic health-check log line (every 60 seconds): MQTT/database connection status, buffer
  utilization, and time since the last successful write, plus a separate WARN when buffer
  utilization exceeds 80%.
- Structured, human-readable logging (`log/slog` text handler) with per-component tagging
  (`main`, `mqtt`, `database`) and UTC timestamps.
- Statically-linked, minimal production Docker image (~18.5 MB, measured), running as a
  non-root user.

## Architecture

```text
                    ┌─────────────────────┐
                    │     MQTT Broker      │
                    │     (Mosquitto)      │
                    └──────────┬───────────┘
                               │ publishes to any topic
                               ▼
                    ┌─────────────────────┐
                    │   MQTT Client        │  Paho callback goroutine
                    │  (internal/mqtt)      │  wildcard subscription "#"
                    └──────────┬───────────┘
                               │ send (blocks if buffer is full)
                               ▼
                    ┌─────────────────────┐
                    │  Buffered Channel     │  chan mqtt.Message
                    │  cap = BUFFER_SIZE    │  default 1000
                    └──────────┬───────────┘
                               │ receive
                               ▼
                    ┌─────────────────────┐
                    │  DB Writer Goroutine  │  dbWriterLoop()
                    │  (cmd/mqtt2bdd)       │  retries every 10s on failure
                    └──────────┬───────────┘
                               │ INSERT ... ON CONFLICT DO NOTHING
                               ▼
                    ┌─────────────────────┐
                    │      PostgreSQL       │
                    │   sensor_metrics      │
                    └─────────────────────┘

  Main goroutine: initializes config/logger/mqtt/database, starts the two
  goroutines above plus a third (Health Check, below), then blocks on
  SIGTERM/SIGINT and coordinates the drain-then-exit shutdown sequence.

  Health Check goroutine: every 60s, reads channel length/capacity and both
  clients' connection status (no writes, no sends) and logs one status line;
  logs a separate WARN if the buffer is above 80% utilization.
```

The application runs three goroutines started from `main()` — the main goroutine itself, the
database writer (`dbWriterLoop`), and the health check (`healthCheckLoop`) — plus a fourth
concurrent path that is not one of the application's own goroutines: the Paho MQTT library's
own callback goroutine, which invokes the message handler each time a message arrives.

The **buffered channel** (`chan mqtt.Message`, capacity `BUFFER_SIZE`) is the sole coupling
point between MQTT reception and database writes. This is Go's "share memory by communicating"
(CSP) pattern in practice: the MQTT callback goroutine and the database writer goroutine never
touch a shared variable directly, they only exchange values over the channel. Backpressure is
intentional, not a bug — when the channel is full, the producer (the MQTT callback) blocks
rather than the process growing memory unboundedly or messages being silently dropped. The
health-check goroutine only **observes** the channel's length and capacity and both clients'
connection status; it never sends to, receives from, or closes the channel, and it never
mutates either client.

## Project Structure

```text
mqtt2bdd/
├── cmd/
│   └── mqtt2bdd/                        # Application entry point
│       ├── main.go                      # Coordinator: goroutines, signals, graceful shutdown
│       ├── main_test.go                 # Unit tests for main.go's pure helpers
│       ├── integration_helpers_test.go  # //go:build integration — shared test scaffolding
│       ├── mqtt_reconnect_integration_test.go  # //go:build integration
│       └── shutdown_integration_test.go        # //go:build integration
├── internal/
│   ├── config/                          # Environment variable loading (LoadConfig)
│   ├── logger/                          # Structured logging wrapper (log/slog)
│   ├── mqtt/                            # MQTT client (Eclipse Paho wrapper)
│   └── database/                        # PostgreSQL client (pgx wrapper)
├── dev/                                 # Development Docker Compose stack
│   ├── docker-compose.yml               # 3 containers: postgres, mosquitto, go-dev (Delve)
│   ├── Dockerfile.dev                   # Go dev image with Delve + staticcheck baked in
│   ├── .env.example                     # Template — copy to .env
│   ├── init-db/01-schema.sql            # Dev database schema init
│   └── mosquitto/mosquitto.conf         # Dev broker config (anonymous access)
├── prod/                                # Production init-db + broker config, used by
│   │                                     # docker-compose.prod.yml (self-contained prod stack)
│   ├── init-db/01-schema.sql
│   └── mosquitto/mosquitto.conf
├── test/                                # Isolated integration-test stack (ephemeral, no persistence)
│   ├── docker-compose.yml
│   ├── init-db/01-schema.sql
│   ├── mosquitto/mosquitto.conf
│   └── run-integration-tests.sh         # Run from the repository root
├── docs/                                # PRD, sharded architecture, stories, QA gates
├── Dockerfile                           # Production multi-stage build
├── docker-compose.prod.yml              # Production deployment stack (app + postgres + mosquitto)
├── .env.prod.example                    # Production env template (no real values)
├── go.mod / go.sum                      # Module: github.com/hagnerk/mqtt2bdd
└── README.md
```

Package responsibilities:

- **`cmd/mqtt2bdd`** — the application's entry point. Wires configuration, logger, MQTT
  client, and database client together; owns the three application goroutines and the
  shutdown sequence.
- **`internal/config`** — loads and validates all configuration from environment variables
  (`LoadConfig()`); the only place `os.Getenv` is called for application settings.
- **`internal/logger`** — a thin wrapper around `log/slog` that resolves the configured log
  level, tags entries with a `component`, and always emits UTC timestamps.
- **`internal/mqtt`** — wraps the Eclipse Paho MQTT client: connecting, the wildcard
  subscription, and automatic reconnection with resubscribe.
- **`internal/database`** — wraps `pgx` for connection pooling and the idempotent
  `INSERT ... ON CONFLICT` used to persist messages.

## Prerequisites

- **Docker** and **Docker Compose** (the `docker compose` plugin, not the standalone
  `docker-compose` binary) — the only hard requirement; the entire development workflow is
  containerized, so no host Go installation is needed to run the application.
- **Go 1.23+** — optional, needed only if you want to run `go` commands directly on the host
  instead of through the containerized `go-dev` toolchain (not how this project's own workflow
  operates, but a reasonable alternative for exploring the code).

This project targets Go ~1.23.6 and Docker Compose ~5.1.2.

## Quick Start

Clone the repository and start the development stack:

```bash
git clone <repository-url>
cd mqtt2bdd/dev
cp .env.example .env
docker compose up --build
```

This starts three containers: `mqtt2bdd-dev-postgres`, `mqtt2bdd-dev-mosquitto`, and
`mqtt2bdd-dev-go-dev`. The `go-dev` container's own command is `dlv debug --headless
--listen=:2345 --api-version=2 --accept-multiclient ./cmd/mqtt2bdd` — **the application starts
automatically under the Delve debugger as soon as the stack is up**; there is no separate
`go run` step to trigger it yourself. Watch its logs with:

```bash
docker compose logs -f go-dev
```

Verify the pipeline end to end by publishing a test message and querying the database:

```bash
# Publish a test message
docker compose exec mosquitto mosquitto_pub -t 'test/temp' -m '{"value": 22.5}'

# Query the database
docker compose exec postgres psql -U mqtt2bdd -d mqtt2bdd -c "SELECT * FROM sensor_metrics LIMIT 10;"
```

All commands above run from `dev/`.

## Configuration

The application's own environment variables — the complete list, read by
`internal/config/config.go`'s `LoadConfig()`:

| Variable | Required | Default | Description |
| -------- | -------- | ------- | ------------ |
| `MQTT_BROKER` | Yes | — | MQTT broker hostname (no protocol prefix; the client builds `tcp://<host>:<port>`) |
| `MQTT_PORT` | Yes | — | MQTT broker port (integer) |
| `MQTT_USERNAME` | No | `""` (empty) | Broker username; leave unset for an anonymous broker |
| `MQTT_PASSWORD` | No | `""` (empty) | Broker password; only applied when `MQTT_USERNAME` is non-empty |
| `POSTGRES_HOST` | Yes | — | PostgreSQL hostname |
| `POSTGRES_PORT` | Yes | — | PostgreSQL port (integer) |
| `POSTGRES_DB` | Yes | — | Database name |
| `POSTGRES_USER` | Yes | — | Database user |
| `POSTGRES_PASSWORD` | Yes | — | Database password |
| `LOG_LEVEL` | No | `INFO` | `DEBUG` \| `INFO` \| `ERROR`, case-insensitive; an empty value silently defaults to INFO, a non-empty unrecognized value also defaults to INFO **and** logs a WARN (`event=unknown_log_level`) |
| `BUFFER_SIZE` | No | `1000` | Message channel capacity (integer); the in-memory outage buffer between MQTT reception and database writes |

These variables are read directly by the Go application, in both the `dev` and production
stacks. `dev/.env.example` and `.env.prod.example` each provide a ready-to-copy template with
realistic values.

Docker Compose also reads three additional variables, but only for its own interpolation in
`docker-compose.prod.yml` — the Go application never reads them:

| Variable | Read by | Default | Purpose |
| -------- | ------- | ------- | ------- |
| `VERSION` | Docker Compose | `dev` | Image tag and the value compiled into `main.Version` via `-ldflags` |
| `POSTGRES_HOST_PORT` | Docker Compose | `5432` | Host-side port published for PostgreSQL (external access, e.g. `psql` from the host) |
| `MQTT_HOST_PORT` | Docker Compose | `1883` | Host-side port published for the MQTT broker (where devices publish to) |

> **Note:** `.env.prod.example`'s own `LOG_LEVEL` comment lists a fourth value, `WARN`. That is
> not a real option — `internal/logger/logger.go` recognizes only `DEBUG`, `INFO`, and `ERROR`;
> setting `LOG_LEVEL=WARN` would silently resolve to `INFO` and additionally log a WARN with
> `event=unknown_log_level`. Use one of the three real levels above.

## Development

### Debugging with Delve

Once the dev stack is up (see Quick Start), the application is already running under Delve,
listening on port `2345`. `.vscode/launch.json` defines an "Attach to Go in Docker"
configuration for it — press **F5** in VS Code to attach, set breakpoints, and inspect
variables.

### Running tests

```bash
docker compose exec go-dev go test ./...
```

With coverage:

```bash
docker compose exec go-dev go test -cover ./...
```

### Code quality tools

```bash
docker compose exec go-dev gofmt -l .
docker compose exec go-dev go vet ./...
docker compose exec go-dev staticcheck ./...
```

All of the above run from `dev/`, against the already-running `dev` stack — never on the
host directly.

### Building with version injection

The application reports its version on startup (`version=…` on the `starting` and
`startup_complete` log lines). The value comes from the `main.Version` variable, which
defaults to `dev` and can be overridden at build time via `-ldflags`.

Build with the default version:

```bash
docker compose exec go-dev go build -o mqtt2bdd ./cmd/mqtt2bdd
# logs: version=dev
```

Build with an injected version:

```bash
docker compose exec go-dev go build -ldflags="-X main.Version=0.1.0" -o mqtt2bdd ./cmd/mqtt2bdd
# logs: version=0.1.0
```

Both commands run from `dev/`. Substitute any version string — a Git tag or a commit SHA,
for example — for `0.1.0`.

## Production Deployment

The production stack (`docker-compose.prod.yml`) runs three containers — the application,
PostgreSQL and Mosquitto — on a dedicated `mqtt2bdd-prod-network` bridge network. It is
independent of the development stack in `dev/` and the two can run side by side.

### 1. Configure

```bash
cp .env.prod.example .env.prod
# Edit .env.prod: set POSTGRES_PASSWORD, and VERSION to the version you are deploying.
```

`.env.prod` holds a real password and is gitignored. Never commit it.

### 2. Deploy

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d --build
```

`--build` compiles the binary from the production `Dockerfile` and injects `VERSION`
into it. `--env-file .env.prod` is required on **every** command against this file,
including `ps`, `logs` and `exec`.

### 3. Verify

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod ps
docker logs mqtt2bdd-prod-app --tail 20
```

All three services report `healthy` once started. The application logs
`event=startup_complete version=…` when it has connected to both the broker and the
database and subscribed to `#`.

The application's health check reports whether it still holds a connection to the broker.
It is a **status signal, not self-healing**: Docker restarts a container that *exits*, not
one that reports `unhealthy`. Watch `docker compose ps`, or point an external watchdog at it.

The stack's `stop_grace_period: 30s` (`docker-compose.prod.yml`) matches
`defaultShutdownTimeout` in `cmd/mqtt2bdd/main.go` exactly, and this is not a coincidence: on
`docker compose down`, or any `stop`, the application needs the full 30 seconds to drain its
buffered messages during a database outage before Docker sends `SIGKILL`. If that constant ever
changes in `main.go`, the Compose value must change with it, or a slow shutdown risks losing
buffered messages.

### 4. Stop

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod down
```

`down` stops the stack and keeps the data volumes. Add `-v` to delete the PostgreSQL
data and the broker's persistence store as well — that is destructive.

### Pointing at an existing broker or database

If PostgreSQL or Mosquitto already run elsewhere on your network, set `POSTGRES_HOST`
and `MQTT_BROKER` in `.env.prod` to their hostnames and start only the application:

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d --build mqtt2bdd
```

### Security

The bundled broker accepts **anonymous** connections: anything that can reach the published
MQTT port can publish to any topic, and — because the application subscribes to `#` — write
rows into the database. That is deliberate and matches this project's threat model (a private
home LAN with no public exposure). **Do not expose `MQTT_HOST_PORT` beyond your trusted
network.** To require credentials instead, see the comments in `prod/mosquitto/mosquitto.conf`;
the application already supports `MQTT_USERNAME` and `MQTT_PASSWORD`.

Every environment variable, with its default and whether it is read by the application or by
Docker Compose, is documented inline in `.env.prod.example`.

## Testing

This section is the canonical reference for both unit and integration testing; see
Development above for the same commands in the context of day-to-day iteration.

### Unit tests

Run against the `dev` stack's `go-dev` container — no live broker or database connection is
required for the unit suite itself, only the container that has the Go toolchain:

```bash
docker compose exec go-dev go test ./...
docker compose exec go-dev go test -cover ./...
```

### Integration tests

Run with a single self-contained script, from the **repository root** (not from `test/`):

```bash
./test/run-integration-tests.sh
```

This script builds and starts the isolated `test/` stack, waits for all services to report
`healthy`, runs a smoke publish-then-verify against the compose-managed application, then runs
the full `//go:build integration` Go test suite from a throwaway container, then tears the
whole stack down unconditionally — regardless of success or failure. It needs no manual setup
and exits non-zero on any failure, making it CI-friendly.

### Current test shape

As a learning aid:

- Unit tests live alongside the code they test: `internal/config`, `internal/logger`,
  `internal/mqtt`, `internal/database`, and `cmd/mqtt2bdd` (`main_test.go`).
- Integration tests (`//go:build integration`) live in `internal/database/integration_test.go`
  and three files under `cmd/mqtt2bdd/`: `integration_helpers_test.go`,
  `mqtt_reconnect_integration_test.go`, and `shutdown_integration_test.go`.

## Troubleshooting

- **MQTT connection failures.** `MQTT_BROKER`/`MQTT_PORT` must match the broker's hostname as
  seen *inside* the Docker network, not the host-published port — a common mistake is using
  the dev stack's container name (`mqtt2bdd-dev-mosquitto`) while pointed at the prod stack, or
  vice versa.
- **PostgreSQL authentication failures.** `POSTGRES_PASSWORD` only takes effect on the
  container's *first* boot against an empty data directory — the `init-db` scripts do not
  re-run on a later password change. Changing `.env`/`.env.prod` after the volume already has
  data will not update the database user's actual password; recreate the volume or change the
  password inside PostgreSQL directly.
- **Forgetting `--env-file .env.prod`.** Every command against `docker-compose.prod.yml` needs
  it — `ps`, `logs`, `exec` included, not just `up` — because the values arrive via Compose
  variable interpolation, which only `--env-file` feeds.
- **`buffer_full` WARN log line.** Means the database is not keeping up with the MQTT broker;
  the application blocks the producer rather than dropping messages, so the operator's window
  to react is the remaining buffer headroom (`BUFFER_SIZE`, default 1000 messages, roughly ten
  minutes of downtime at 100 messages/minute).
- **Health check reports `unhealthy` but the container is not restarted.** This is by design,
  stated in `docker-compose.prod.yml`'s own comment: `restart:` reacts to container *exit*, not
  to `unhealthy` status; watch `docker compose ps` or point an external watchdog at it.
- **Permission errors on the mounted project directory** (`dev/docker-compose.yml`'s `../:/app`
  bind mount) — typically a Docker Desktop file-sharing setting on macOS/Windows; ensure the
  repository's parent directory is within Docker's allowed file-sharing paths.

## Learning Resources

Curated, official/canonical links related to the technologies and patterns this project uses:

### Go language & stdlib

- [The Go Tour](https://go.dev/tour/) — the official interactive Go tour, the natural starting
  point for a Go beginner.
- [Effective Go](https://go.dev/doc/effective_go) — idioms this codebase follows throughout
  (naming conventions, error handling, early returns).
- [`log/slog` package docs](https://pkg.go.dev/log/slog) — the structured logging package
  `internal/logger` wraps.
- [`context` package docs](https://pkg.go.dev/context) — the cancellation/timeout pattern used
  throughout `cmd/mqtt2bdd/main.go`.
- [Go Concurrency Patterns: Pipelines and cancellation](https://go.dev/blog/pipelines) — the
  concurrency pattern (goroutines + channels) this project's MQTT-to-database pipeline
  demonstrates directly.

### MQTT

- [mqtt.org](https://mqtt.org/) — the protocol's official site.
- [OASIS MQTT 5.0 specification](https://docs.oasis-open.org/mqtt/mqtt/v5.0/mqtt-v5.0.html) —
  the official protocol specification.
- [`paho.mqtt.golang` package docs](https://pkg.go.dev/github.com/eclipse/paho.mqtt.golang) —
  the Go client library this project wraps in `internal/mqtt`.

### PostgreSQL

- [PostgreSQL documentation](https://www.postgresql.org/docs/) — official documentation.
- [JSON types](https://www.postgresql.org/docs/current/datatype-json.html) — JSONB, the type
  `sensor_metrics.metrics` uses.
- [`pgx/v5` package docs](https://pkg.go.dev/github.com/jackc/pgx/v5) — the driver this
  project's `internal/database` wraps.

### Docker Compose

- [Docker Compose documentation](https://docs.docker.com/compose/) — official documentation
  for the `docker compose` command this project's entire workflow depends on.
