# Infrastructure and Deployment

Defining the deployment architecture for MQTT2BDD's on-premises Proxmox deployment with containerized development environments.

## Infrastructure as Code

**Tool:** Docker Compose ~5.1.2

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

## Deployment Strategy

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

## Environments

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
  - `mqtt2bdd-test-mqtt2bdd` (Application under test)
- **Network:** `mqtt2bdd-test-network` (isolated, no conflicts with dev)
- **Ports:** PostgreSQL and Mosquitto ports are published and parameterised, defaulting to
  `5433` and `1884` to avoid colliding with the dev stack's `5432`/`1883` (Story 3.3, AC4).
  This is for manual/operator access — `psql`/`mosquitto_pub` against the running stack
  from the host, or from `go-dev` via `host.docker.internal`. The Go integration test
  suite itself reaches the stack over the internal network, from a separate throwaway
  container attached directly to `mqtt2bdd-test-network`, so it never depends on these
  published ports.
  - PostgreSQL accessible at `mqtt2bdd-test-postgres:5432` (internal) or `localhost:5433` (host)
  - Mosquitto accessible at `mqtt2bdd-test-mosquitto:1883` (internal) or `localhost:1884` (host)
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
    container_name: mqtt2bdd-test-mqtt2bdd
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

> **Note (Story 3.3):** the `docker-compose exec -T mqtt2bdd-test-app go test ...` invocation above
> predates this story's actual implementation and would fail against the shipped image — the `mqtt2bdd`
> service is built from the production `Dockerfile`'s runtime stage, which has no Go toolchain (established
> as fact when Story 3.1/3.2 closed FIND-024). The real Go integration suite runs in a separate, throwaway
> `golang:1.23-alpine` container instead, per `test/run-integration-tests.sh`.

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

> **Note (Story 3.2):** the `docker-compose.prod.yml` that ships is a **self-contained three-service
> stack** — `mqtt2bdd-prod-app`, `mqtt2bdd-prod-postgres` and `mqtt2bdd-prod-mosquitto` — as PRD Epic 3,
> Story 3.2 AC1 and AC5 require, with `prod/init-db/01-schema.sql` and `prod/mosquitto/mosquitto.conf`
> supplying the database schema and broker configuration. The app-only deployment against externally
> managed PostgreSQL and Mosquitto described below — including the **External Services Prerequisites** and
> **Rationale for External Services** subsections — remains supported and is reached by setting
> `POSTGRES_HOST` and `MQTT_BROKER` in `.env.prod` to the external hostnames and starting the application
> service alone (`docker compose -f docker-compose.prod.yml --env-file .env.prod up -d mqtt2bdd`). It is
> a documented alternative, not what the file defines by default.

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

## Environment Promotion Flow

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

## Rollback Strategy

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

## Backup Strategy

**Database Backups (External PostgreSQL):**

Since PostgreSQL is managed externally, backups are handled at the PostgreSQL server level (not in MQTT2BDD scope). Coordinate with Proxmox administrator for:

- Daily automated backups
- Retention policy (30 days daily, 3 months weekly, 1 year monthly)
- Backup verification
- Restore procedures

> **Note (Story 3.2):** this applies to the app-only deployment against an externally managed PostgreSQL.
> In the self-contained three-service stack that ships, the database lives in the `mqtt2bdd-prod-pgdata`
> named volume on the deployment host, and backing that volume up — or dumping from
> `mqtt2bdd-prod-postgres` — is in scope for whoever operates the stack.

**Application State:**
- No persistent application state (stateless)
- Configuration in `.env.prod` (version-controlled)
- Docker images versioned and tagged

## Monitoring and Observability

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
