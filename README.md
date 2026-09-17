# MQTT2BDD

![Go Version](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go&logoColor=white) [![License: MIT](https://img.shields.io/badge/License-MIT-yellow)](LICENSE)

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
- Automatic insert retry on database outage — a message that fails for a transient reason (connection lost, database unavailable, missing privilege) is retried every 10 seconds until it succeeds, so no message is lost during an outage. A message the database can never accept (invalid JSON, or refused by PostgreSQL with a data, constraint or limit error) is discarded and logged at ERROR as `write_rejected` instead of retried, so it cannot stall the messages behind it; empty payloads (MQTT's way of clearing a retained message) are skipped.
- Repair of content PostgreSQL would refuse but that can be fixed without guesswork — a JSON `\u0000` escape, an unpaired UTF-16 surrogate escape (a lone `\ud83d`), invalid UTF-8 bytes: each defect is replaced with U+FFFD (`�`), the message is stored, and one WARN `payload_sanitized` line reports it. **The stored JSON may therefore differ from the published payload** in exactly that way (and, as always with `jsonb`, in key order, duplicate keys and whitespace). Everything else PostgreSQL accepts, such as `\\u0000` (literal text) or a valid surrogate pair, is stored as published.
- Idempotent writes via `INSERT ... ON CONFLICT (sensor, date) DO NOTHING`.
- Graceful shutdown on SIGTERM/SIGINT: stops the MQTT client, drains all buffered messages
  (bounded by a 30-second timeout), then closes the database pool in order.
- Periodic health-check log line (every 60 seconds): MQTT/database connection status, buffer utilization, time since the last successful write, and messages written and rejected since the previous line (`processed_last_interval`, `rejected_last_interval`), plus a separate WARN when buffer utilization exceeds 80%.
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

  Main goroutine: initializes config/logger/mqtt/database, starts the DB Writer
  goroutine above plus a second one (Health Check, below), then blocks on
  SIGTERM/SIGINT and coordinates the drain-then-exit shutdown sequence. The MQTT
  Client box above is the Paho library's own callback goroutine, not one main()
  starts itself.

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
│       ├── mqtt_reconnect_integration_test.go   # //go:build integration
│       ├── payload_repair_integration_test.go   # //go:build integration
│       ├── shutdown_integration_test.go         # //go:build integration
│       └── write_rejection_integration_test.go  # //go:build integration
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
├── .github/
│   └── workflows/release.yml            # Tag-triggered build of the four release binaries
├── Dockerfile                           # Production multi-stage build
├── docker-compose.prod.yml              # Production deployment stack (app + postgres + mosquitto)
├── .env.prod.example                    # Production env template (no real values)
├── go.mod / go.sum                      # Module: github.com/hagnerk/mqtt2bdd
├── LICENSE                              # MIT
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
  `docker-compose` binary) — the only hard requirement **for developing on this project**; the
  entire development workflow is containerized, so no host Go installation is needed. To *run*
  the application you need neither Docker nor Go: download a release binary instead, as
  described under [Installation](#installation).
- **Go 1.23+** — optional, needed only if you want to run `go` commands directly on the host
  instead of through the containerized `go-dev` toolchain (not how this project's own workflow
  operates, but a reasonable alternative for exploring the code).

This project targets Go ~1.23.6 and Docker Compose ~5.1.2.

## Installation

**This is the path for running MQTT2BDD.** If you want to work on the code instead, skip to
[Quick Start](#quick-start).

Every tagged version publishes statically-linked binaries for four platforms. Downloading one
needs no Go toolchain, no Docker, and no build step.

### Pick your archive

Run `uname -m` on the target machine and read the answer off this table. This is the step people
get wrong: a Raspberry Pi 4 can run either a 32-bit or a 64-bit OS, and what matters is the OS
you installed, not what the hardware could support.

| `uname -m` | Platform | Canonical archive | Version-free alias |
| --- | --- | --- | --- |
| `x86_64` | Linux on 64-bit Intel/AMD | `mqtt2bdd_<version>_linux_amd64.tar.gz` | `mqtt2bdd_linux_amd64.tar.gz` |
| `aarch64` | Linux on 64-bit ARM — **64-bit** Raspberry Pi OS, most ARM servers | `mqtt2bdd_<version>_linux_arm64.tar.gz` | `mqtt2bdd_linux_arm64.tar.gz` |
| `armv7l` | Linux on 32-bit ARM — **32-bit** Raspberry Pi OS | `mqtt2bdd_<version>_linux_armv7.tar.gz` | `mqtt2bdd_linux_armv7.tar.gz` |
| `arm64` (macOS) | macOS on Apple Silicon | `mqtt2bdd_<version>_darwin_arm64.tar.gz` | `mqtt2bdd_darwin_arm64.tar.gz` |

There is deliberately **no `darwin_amd64` build**: Intel Macs are not a target. Build from source
if you need one — the application cross-compiles with no special toolchain.

Each archive contains three files and no enclosing directory: the `mqtt2bdd` binary (already
executable), `LICENSE`, and `README.md`.

### Download

Two URL forms exist, and the choice matters more than it looks.

**Pinned** — use this for anything scripted, deployed, or written down. The tag is part of the
URL, so it keeps serving that exact version forever. A later `2.0.0` with breaking configuration
changes can never appear underneath a script that pinned:

```bash
curl -fLO https://github.com/hagnerk/mqtt2bdd/releases/download/v1.2.3/mqtt2bdd_1.2.3_linux_amd64.tar.gz
```

**Latest** — use this for a first manual try. It follows the newest non-pre-release version
automatically, which is convenient exactly until it is not: the version can change under you
between one run and the next. This is why the alias archives carry no version in their names —
the `latest` URL requires an asset name that stays the same across releases:

```bash
curl -fLO https://github.com/hagnerk/mqtt2bdd/releases/latest/download/mqtt2bdd_linux_amd64.tar.gz
```

`latest` never points at a pre-release. While only release candidates have been published, every `latest` URL returns 404: use the pinned form with the candidate's tag instead.

The `-f` in every `curl` command in this section is deliberate. Without it, an HTTP error such as a 404 is saved under the archive's name and `curl` still exits 0, so the failure only surfaces later, as a confusing error from `tar` about the archive format. With it, `curl` stops at the step that actually failed.

### Verify the download

Fetch `checksums.txt` **from the same release you downloaded the archive from** — a checksum file
from a different version will not match anything you have:

```bash
# If you took the pinned URL above:
curl -fLO https://github.com/hagnerk/mqtt2bdd/releases/download/v1.2.3/checksums.txt

# If you took the latest URL above:
curl -fLO https://github.com/hagnerk/mqtt2bdd/releases/latest/download/checksums.txt
```

Then verify, naming whichever archive you actually downloaded:

```bash
sha256sum --ignore-missing -c checksums.txt
# mqtt2bdd_1.2.3_linux_amd64.tar.gz: OK      <- pinned download
# mqtt2bdd_linux_amd64.tar.gz: OK            <- latest download
```

Either works without you having to know about the other: `checksums.txt` lists every archive under
**both** its canonical and its alias name, and the two are byte-identical, so they share a checksum.

`--ignore-missing` is not optional here. `checksums.txt` lists all eight archives — four platforms
under two names each — and you have downloaded one. Without the flag, `sha256sum` reports seven
failures for files that were never meant to be there and exits non-zero, which reads like a failed
verification rather than an absent file.

On Alpine or anywhere else with BusyBox, `sha256sum` has no `--ignore-missing`. Select your line
instead:

```bash
grep mqtt2bdd_1.2.3_linux_amd64.tar.gz checksums.txt | sha256sum -c -
```

A matching checksum tells you the download is complete and uncorrupted. It is not a signature and
proves nothing about who produced the file.

### Extract and run

Substitute the archive name you downloaded — `mqtt2bdd_linux_amd64.tar.gz` if you took the latest
URL. The contents are identical either way:

```bash
tar -xzf mqtt2bdd_1.2.3_linux_amd64.tar.gz
./mqtt2bdd
# time=... level=ERROR msg="configuration error" component=main event=config_load_failed operation=load_config error="required environment variable MQTT_BROKER is not set"
```

That error is the expected result of a first run, not a broken download: the binary works, it simply has not been told where its broker and database are yet — see [What the binary still needs](#what-the-binary-still-needs). Once the required variables are set, the first line reports the version the binary was built from:

```text
time=... level=INFO msg="MQTT2BDD starting" component=main event=starting version=1.2.3
```

The Linux binaries are statically linked (`CGO_ENABLED=0` — no libc, no dynamic loader), so they
have no runtime dependency at all: no package to install, no container to run them in.

**On macOS, the binary is unsigned.** A browser marks what it downloads as quarantined, `tar` carries that mark over to the extracted binary, and Gatekeeper then refuses to run it with a dialog that suggests the file is damaged. It is not. Clear the quarantine attribute once:

```bash
xattr -d com.apple.quarantine ./mqtt2bdd 2>/dev/null || true
```

`curl` does not set the quarantine attribute, so after a `curl` download there is usually nothing to clear; `2>/dev/null || true` keeps the command harmless either way.

### What the binary still needs

A binary alone is not a working system. Before the first run you need:

- **A reachable MQTT broker.** Any broker; the application subscribes to `#` and needs no
  per-sensor configuration.
- **A PostgreSQL database using the `UTF8` encoding, with the `sensor_metrics` schema applied.** The schema is [`prod/init-db/01-schema.sql`](prod/init-db/01-schema.sql) — apply it once with `psql -f prod/init-db/01-schema.sql`. Under any other encoding, PostgreSQL refuses (SQLSTATE `22P05`) every character that encoding cannot represent, whether escaped or not, including the U+FFFD written by the payload repair, and those messages are discarded. Check it with `SHOW server_encoding;`; the official `postgres` image used by the bundled stacks initialises with `UTF8`.
- **Environment variables** telling the application where those two are. Every variable, its default, and whether it is required is in [Configuration](#configuration). Without them the application exits immediately with `event=config_load_failed`, naming the first required variable it could not find. With them set but the broker or the database unreachable, it logs `event=starting`, then `event=startup_aborted`, and exits non-zero. Those two are the most common reasons a first run "does not work".

### Pinning to a version range

GitHub resolves **no** semver ranges server-side. There is no `^1.3` or `~1.3` URL; the only two
forms the server understands are a pinned tag and `latest`. Anything in between is the client's
job.

If a script needs "the newest 1.x", list the releases and pick the tag yourself, then build the
pinned URL from it:

```bash
TAG=$(curl -fs https://api.github.com/repos/hagnerk/mqtt2bdd/releases \
  | jq -r '.[] | select(.prerelease | not) | .tag_name' \
  | grep '^v1\.' | head -1)
[ -n "$TAG" ] || { echo "no matching release" >&2; exit 1; }
curl -fLO "https://github.com/hagnerk/mqtt2bdd/releases/download/${TAG}/mqtt2bdd_${TAG#v}_linux_amd64.tar.gz"
```

The guard line matters. When nothing matches, `TAG` is empty, and without the guard the script would build a URL with no tag in it and download nothing.

That endpoint is unauthenticated and therefore rate-limited to 60 requests per hour per IP. Past the limit it answers with an error instead of a release list: `-f` makes that `curl` fail, `TAG` comes out empty, and the same guard stops the script. A script that runs often should still cache the resolved tag.

## Quick Start

**This is the path for developing on MQTT2BDD.** To just run it, see
[Installation](#installation) above.

Clone the repository and start the development stack:

```bash
git clone https://github.com/hagnerk/mqtt2bdd.git
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

### Cutting a release

Releases are produced entirely by `.github/workflows/release.yml`. There is no manual build step,
and the version a released binary reports comes from the tag by the same `-ldflags` mechanism
described just above.

```bash
git tag v1.2.3
git push origin v1.2.3
```

That is the whole process. On a tag matching `v*.*.*`, the workflow runs this project's existing
quality gates first — `gofmt`, `go vet`, `staticcheck`, `go test`, and the full integration suite —
and only if every one of them passes does it build the four platforms and publish a release with
nine assets: eight archives and `checksums.txt`.

If a gate fails, nothing is published. The tag and its commits stay where they are; only the
release is blocked. To recover, fix the problem, remove the tag from both places, and re-tag:

```bash
git tag -d v1.2.3
git push --delete origin v1.2.3
```

**Rehearse on a pre-release tag first.** Tag `v1.2.3-rc1`, confirm the run succeeded and the nine
assets are present, then tag the real version. Any tag whose version contains a `-` is published
as a pre-release, and a pre-release never becomes the target of the `latest` URL — so a rehearsal
cannot be served to somebody who asked for the newest version.

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
- Integration tests (`//go:build integration`) live in `internal/database/integration_test.go` and five files under `cmd/mqtt2bdd/`: `integration_helpers_test.go`, `mqtt_reconnect_integration_test.go`, `payload_repair_integration_test.go`, `shutdown_integration_test.go`, and `write_rejection_integration_test.go`.

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
- **`write_rejected` ERROR log line.** The database can never store that message, so it was discarded; the other messages keep flowing. `topic`, `payload_size`, `reason` (`invalid_json` or `sqlstate`) and `sqlstate` identify it; its body is on the DEBUG `message_received` line. By contrast, a `write_failure` repeating every 10 seconds points at the environment (connection, privilege, missing table) and is retried on purpose until it is fixed.
- **`payload_sanitized` WARN log line.** The message was stored, but with U+FFFD (`�`) in place of each piece of content PostgreSQL would have refused. `nul_escapes`, `lone_surrogates` and `invalid_utf8_sequences` count each kind of repair; `payload_size` is the size as received. The publisher is sending malformed data: fix it at the source if that data matters.
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

## License

MQTT2BDD is released under the MIT License. See [LICENSE](LICENSE) for the full text.
