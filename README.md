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
