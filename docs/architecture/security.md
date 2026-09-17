# Security

## Threat Model

MQTT2BDD is deployed on a **private home network** (Proxmox server) with no public internet exposure. There is no HTTP server, no user-facing API, and no web interface. This significantly reduces the attack surface compared to a typical web application.

**Primary threats in scope:**
- Leaked credentials (secrets committed to Git or exposed via logs)
- SQL injection via malformed MQTT payloads
- Privilege escalation via container running as root
- Compromised Go dependency in the supply chain

**Threats explicitly out of scope** for this deployment:
- External network attacks (no public endpoint)
- Authentication/authorisation bypass (no user sessions, no API)
- XSS, CSRF, session hijacking (no web interface)
- DDoS (private network, no public exposure)

This threat model is honest about the project's context. A deployment exposed to the internet would require a substantially different security posture (TLS on all connections, firewall rules, rate limiting, etc.).

## Secrets Management

**Rule: No secret ever touches the Git repository.**

Credentials and sensitive configuration are passed exclusively via environment variables at runtime:

| Secret | Environment Variable | Storage |
|--------|---------------------|---------|
| PostgreSQL password | `POSTGRES_PASSWORD` | `.env.prod` (not committed) |
| MQTT password (optional) | `MQTT_PASSWORD` | `.env.prod` (not committed) |

**`.gitignore` entries (mandatory):**
```gitignore
# Environment files with secrets — never commit
.env
.env.prod
*.env

# Editor and OS artifacts
.DS_Store
.vscode/settings.json  # may contain local paths
```

**Template file committed instead:**
```bash
# .env.prod.example — committed to Git, contains no real values
POSTGRES_PASSWORD=<replace_with_strong_password>
MQTT_PASSWORD=<replace_if_broker_requires_auth>
```

**In code:** Secrets are never logged in plain text. Passwords are replaced with `***` to confirm they were loaded, or `<not set>` to surface a missing optional credential early:

```go
logger.Info("Configuration loaded",
    "mqtt_broker",         cfg.MQTTBroker,
    "mqtt_port",           cfg.MQTTPort,
    "mqtt_password",       maskSecret(cfg.MQTTPassword),
    "postgres_host",       cfg.PostgresHost,
    "postgres_db",         cfg.PostgresDB,
    "postgres_password",   maskSecret(cfg.PostgresPassword),
)

// maskSecret returns "***" if the secret is set, "<not set>" if empty.
// Use for logging credentials: confirms presence without exposing the value.
func maskSecret(s string) string {
    if s == "" {
        return "<not set>"
    }
    return "***"
}
```

Example startup log output:
```
INFO Configuration loaded mqtt_broker=mosquitto.local mqtt_port=1883 mqtt_password=*** postgres_host=postgres.local postgres_db=mqtt2bdd postgres_password=***
```

## SQL Injection Prevention

All database operations use **parameterized queries** via pgx — user-controlled data (MQTT topic, payload) is never interpolated directly into SQL strings:

```go
// Correct — parameterized, safe against injection
query := `INSERT INTO sensor_metrics (sensor, date, metrics) VALUES ($1, $2, $3)`
c.pool.Exec(ctx, query, sensor, timestamp, metrics)

// Incorrect — never do this
query := fmt.Sprintf("INSERT INTO sensor_metrics ... VALUES ('%s', ...)", sensor)
```

pgx automatically handles escaping for all parameter types. Since the application uses `pool.Exec()` with positional parameters (`$1`, `$2`, `$3`) for every write, SQL injection via MQTT topic names or JSON payloads is not possible.

## Input Validation

MQTT payloads are treated as **untrusted input**. The MQTT client forwards every message to the buffer unchanged; validation happens in the database layer, in `InsertMessage` (`internal/database/queries.go`), just before the INSERT:

```go
// An empty payload is skipped: nothing to store, not an error.
if len(metrics) == 0 {
    c.logger.Debug("empty payload skipped", "event", "write_skipped", "topic", sensor, "reason", "empty_payload")
    return nil
}

// A payload that is not JSON is rejected without contacting the database.
if !json.Valid(metrics) {
    rejectErr := fmt.Errorf("%w: invalid JSON", ErrRejected)
    c.recordRejection(sensor, len(metrics), 0, rejectErr, "reason", "invalid_json")
    return rejectErr
}
```

A rejected message is logged once, at ERROR, as `write_rejected`, with `topic` and `payload_size` but **never the payload body**: the body is untrusted, can be hundreds of kilobytes, and is already available on the DEBUG `message_received` entry when an operator needs it. A rejected message is discarded, not retried (see [Error Handling Strategy §3](./error-handling-strategy.md#3-runtime-errors---data-integrity-log-and-skip)).

**What is validated:**
- Payload is non-empty (an empty payload is skipped, not rejected)
- JSON syntax, locally (via `json.Valid()`)
- Content PostgreSQL can store, server-side: a payload that passes `json.Valid` but that PostgreSQL refuses with a data, constraint or limit error (SQLSTATE class 22, 23 or 54, such as a `\u0000` escape or a number beyond the `numeric` range) is rejected the same way, with `reason=sqlstate`

**What is not validated (intentional):**
- JSON schema/field names — MQTT2BDD is device-agnostic by design; field validation is Grafana's responsibility at query time
- Topic format — wildcard subscription captures everything; rejecting topics by pattern would break device-agnostic behaviour

## Container Security

The production Docker image applies three hardening measures:

**1. Non-root user**

The container runs as an unprivileged user, limiting the blast radius of a compromised process. The image sets `USER nobody` in the runtime stage of the Dockerfile, so the container is non-root by default wherever it is deployed:

```dockerfile
# Dockerfile (runtime stage)
FROM alpine:3.23
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/mqtt2bdd /app/mqtt2bdd
USER nobody
ENTRYPOINT ["/app/mqtt2bdd"]
```

`nobody` resolves to UID `65534` and GID `65534` on Alpine. `docker-compose.prod.yml` may still pin the same identity explicitly, and the two agree numerically:

```yaml
# docker-compose.prod.yml
services:
  app:
    user: "65534:65534"  # nobody:nogroup — standard unprivileged user on Alpine/Linux
```

UID `65534` (`nobody`) and GID `65534` (`nogroup`) are conventionally reserved for unprivileged processes on Linux and Alpine. `USER` in the Dockerfile is a default, not a lock: `docker run --user` and Compose's `user:` key both override it, so the image stays portable while remaining safe when the orchestration layer says nothing.

> **Note:** If the binary needs to read a file owned by root (e.g. a mounted secret file), ensure the file permissions allow read by UID 65534, or adjust the `user:` value to match your host environment's unprivileged UID.

**2. Minimal base image**

The runtime stage uses `alpine:3.23` (8,657,696 bytes, ≈8.7 MB), not `golang:alpine` (~300 MB); the image built from it measures 18,554,844 bytes. The final image contains the statically-linked binary, a CA trust store and Alpine's minimal userland — no Go toolchain and no application source.

Alpine's userland is minimal, not empty: it ships busybox 1.37.0 (304 applets, including `/bin/sh`) and apk-tools 3.0.6, so both a shell and a package manager are present in the runtime image and `docker run --entrypoint sh` resolves. What constrains them is `USER nobody` and the read-only root filesystem below, not their absence.

**3. Read-only filesystem (recommended)**

The application writes nothing to disk (logs go to stdout, no state files). The container can be run with a read-only root filesystem:

```yaml
# docker-compose.prod.yml
services:
  app:
    read_only: true
    tmpfs:
      - /tmp  # required by some Go runtime internals
```

## Dependency Security

Go modules provide built-in supply chain protection:

**`go.sum` file:** Cryptographic checksums for every dependency and its transitive dependencies. Any tampered module will fail checksum verification at build time.

**Verify integrity at any time:**
```bash
go mod verify  # Checks all cached modules against go.sum
```

**Keep dependencies minimal and updated:**
- Only two external dependencies: `paho.mqtt.golang` and `pgx/v5`
- Dependency updates reviewed manually before merging (small surface area makes this practical)

**Vulnerability scanning — two complementary tools:**

1. **`govulncheck`** (official Go vulnerability checker): queries the Go vulnerability database (`vuln.go.dev`) and reports only vulnerabilities that are **reachable in your code** — not every CVE present in the dependency tree. This eliminates false positives. Added to the CI pipeline:
   ```yaml
   - name: Vulnerability scan
     run: |
       go install golang.org/x/vuln/cmd/govulncheck@latest
       govulncheck ./...
   ```

2. **Dependabot** (GitHub native): monitors `go.mod` continuously and opens automated PRs when a new vulnerability is published for a dependency, without waiting for the next push. Enable via `.github/dependabot.yml`:
   ```yaml
   version: 2
   updates:
     - package-ecosystem: gomod
       directory: "/"
       schedule:
         interval: weekly
   ```

**No `replace` directives in `go.mod`** that could silently swap a dependency for a local or forked version.

## Network Security

**Current posture (private network):**
- Connections to MQTT broker and PostgreSQL are unencrypted (plain TCP)
- Acceptable for a home network where all services are on the same LAN/VLAN
- Credentials still protected via environment variables even without TLS

**Upgrade path to TLS (if network perimeter changes):**

Both Paho and pgx support TLS natively. Enabling it requires:

```go
// MQTT with TLS
tlsConfig := &tls.Config{InsecureSkipVerify: false}  // verify server cert
opts.SetTLSConfig(tlsConfig)
opts.AddBroker(fmt.Sprintf("tls://%s:%d", cfg.MQTTBroker, cfg.MQTTTLSPort))

// PostgreSQL with TLS (via connection string)
// POSTGRES_DSN=postgres://user:pass@host:5432/db?sslmode=verify-full
```

TLS is not enabled by default to avoid operational complexity on the home network. It should be enabled if the deployment ever spans untrusted network segments.

## MQTT Broker Authentication (Accepted Risk, Not a Story)

**Current posture:** `prod/mosquitto/mosquitto.conf` sets `allow_anonymous true`, with `MQTT_HOST_PORT`
published to the host. `dev/mosquitto/mosquitto.conf` has carried the identical posture since Story 1.1.
Anyone who can reach the published port can publish and subscribe to any topic, and — because the
application subscribes to `#` — write rows into `sensor_metrics` with no credential.

**Why this is recorded as an accepted risk rather than assigned to a story.** The posture is consistent
with the Threat Model above, which already places this deployment on a private home LAN with no public
exposure and lists "Authentication/authorisation bypass" as explicitly out of scope. It was reviewed and
deliberately deferred at PO validation for both Story 3.1 and Story 3.2 (`docs/stories/3.1.story.md`,
`docs/stories/3.2.story.md` → PO Validation) and confirmed sound both times by QA. Neither deferral gave it
a home, because none of Epic 3's remaining approved stories fits: 3.3 is an integration-test harness, 3.4
is unit test coverage, 3.5 is the README rewrite, and 3.6 is inline code documentation — none changes
broker behaviour, and an unapproved seventh Epic 3 story is not created here or by any prior story.

**The escape hatch already ships and works today**, so this note blocks nothing: `internal/config/config.go:83-84`
reads `MQTT_USERNAME` / `MQTT_PASSWORD`, `internal/mqtt/client.go:53-55` applies them when the username is
non-empty, and `.env.prod.example` documents both. Enabling authentication requires only a
`mosquitto_passwd` credential file, `password_file` and `allow_anonymous false` in
`prod/mosquitto/mosquitto.conf`, and setting the two variables in `.env.prod` — no application code changes.

**Revisit trigger:** the day this stack is deployed anywhere the published MQTT port is reachable from
outside a trusted LAN, not on a fixed schedule. Until that condition holds, this is an accepted risk against
a documented threat model, not a deferred task waiting on a story number.

## Security Checklist (Pre-Deployment)

- [ ] `.env.prod` is not committed to Git
- [ ] `POSTGRES_PASSWORD` is a strong, unique password (not reused)
- [ ] Container runs as non-root user (`USER nobody` in Dockerfile)
- [ ] `go mod verify` passes with no errors
- [ ] No secrets appear in application logs (check with `docker logs | grep -i password`)
- [ ] PostgreSQL user `mqtt2bdd` has only `INSERT` and `SELECT` privileges (principle of least privilege):
  ```sql
  GRANT INSERT, SELECT ON sensor_metrics TO mqtt2bdd;
  -- Do NOT grant DROP, ALTER, or TRUNCATE
  ```

---
