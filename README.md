# MQTT2BDD

## Overview

MQTT2BDD is a Go application that bridges home automation MQTT messages into a PostgreSQL database for persistent storage and analysis. It subscribes to all topics on a configured MQTT broker using a wildcard subscription and inserts each message's topic, timestamp, and JSON payload into a `sensor_metrics` table, enabling historical analysis and Grafana visualization of device data.

## Project Structure

```
mqtt2bdd/
├── cmd/
│   └── mqtt2bdd/           # Application entry point (main.go)
├── internal/
│   ├── config/             # Environment variable configuration loading
│   ├── logger/             # Structured logging wrapper (log/slog)
│   ├── mqtt/               # MQTT client (Eclipse Paho wrapper)
│   └── database/           # PostgreSQL client (pgx wrapper)
├── pkg/                    # Public reusable libraries (future use)
├── dev/                    # Development Docker Compose stack
│   ├── docker-compose.yml  # Three containers: PostgreSQL, Mosquitto, Go dev
│   ├── Dockerfile.dev      # Go dev image with Delve + staticcheck
│   ├── init-db/            # PostgreSQL initialization scripts
│   └── mosquitto/          # Mosquitto broker configuration
├── go.mod                  # Go module definition
└── README.md               # This file
```

## Getting Started

The development environment is fully containerized — no local Go installation required.

**Prerequisites:** Docker and Docker Compose.

```bash
cd dev/
cp .env.example .env
docker-compose up --build
```

This starts three containers: `mqtt2bdd-dev-postgres`, `mqtt2bdd-dev-mosquitto`, and `mqtt2bdd-dev-go-dev` (with Delve remote debugger on port 2345).

See Story 1.1 for full setup details and VS Code debugging configuration.

## Build

The application reports its version on startup (`version=…` on the `starting` and
`startup_complete` log lines). The value comes from the `main.Version` variable, which
defaults to `dev` and is overridden at build time via ldflags.

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
