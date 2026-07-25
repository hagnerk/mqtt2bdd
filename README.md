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

## Production Deployment

The production stack (`docker-compose.prod.yml`) runs three containers — the application,
PostgreSQL and Mosquitto — on a dedicated `mqtt2bdd-prod-network` bridge network. It is
independent of the development stack in `dev/` and the two can run side by side.

**1. Configure**

```bash
cp .env.prod.example .env.prod
# Edit .env.prod: set POSTGRES_PASSWORD, and VERSION to the version you are deploying.
```

`.env.prod` holds a real password and is gitignored. Never commit it.

**2. Deploy**

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d --build
```

`--build` compiles the binary from the production `Dockerfile` and injects `VERSION`
into it. `--env-file .env.prod` is required on **every** command against this file,
including `ps`, `logs` and `exec`.

**3. Verify**

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

**4. Stop**

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod down
```

`down` stops the stack and keeps the data volumes. Add `-v` to delete the PostgreSQL
data and the broker's persistence store as well — that is destructive.

**Pointing at an existing broker or database**

If PostgreSQL or Mosquitto already run elsewhere on your network, set `POSTGRES_HOST`
and `MQTT_BROKER` in `.env.prod` to their hostnames and start only the application:

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d --build mqtt2bdd
```

**Security**

The bundled broker accepts **anonymous** connections: anything that can reach the published
MQTT port can publish to any topic, and — because the application subscribes to `#` — write
rows into the database. That is deliberate and matches this project's threat model (a private
home LAN with no public exposure). **Do not expose `MQTT_HOST_PORT` beyond your trusted
network.** To require credentials instead, see the comments in `prod/mosquitto/mosquitto.conf`;
the application already supports `MQTT_USERNAME` and `MQTT_PASSWORD`.

**Configuration reference:** every environment variable, with its default and whether
it is read by the application or by Docker Compose, is documented inline in
`.env.prod.example`.
