# Tech Stack

This is the **DEFINITIVE technology selection section** - all other documents and agents will reference these choices as the single source of truth.

## Cloud Infrastructure

- **Provider:** Docker-based deployment (infrastructure-agnostic, deployable on any Docker host)
- **Key Services:** None - self-hosted PostgreSQL and Mosquitto via Docker Compose
- **Deployment Regions:** On-premises (home automation infrastructure, Proxmox server)

_Note: This is an on-premises deployment, not cloud-based. The architecture is cloud-ready (can deploy to AWS ECS, GCP Cloud Run, etc.) but targets local infrastructure per PRD requirements._

## Technology Stack Table

| Category | Technology | Version | Purpose | Rationale |
|----------|-----------|---------|---------|-----------|
| **Language** | Go | ~1.23.6 | Primary development language | Latest stable (Feb 2026), native concurrency (goroutines/channels), excellent tooling, strong typing, fast compilation, educational value for learning systems programming |
| **Runtime** | Go Runtime | ~1.23.6 | Application execution environment | Statically-linked binary, minimal dependencies, production-ready |
| **MQTT Client** | Eclipse Paho | ~1.5.0 | MQTT protocol implementation | Official Eclipse Foundation library, production-proven, auto-reconnect support, comprehensive QoS handling, excellent Go integration |
| **Database Driver** | pgx | ~5.7.0 | PostgreSQL connectivity | Modern high-performance driver, superior to lib/pq, native connection pooling, excellent error context, prepared statement support |
| **Database** | PostgreSQL | ~15.10 | Persistent data storage | JSONB native support (metrics column), ACID compliance, mature ecosystem, Grafana integration, widely adopted for time-series data |
| **MQTT Broker** | Mosquitto | ~2.0.20 | Message broker (dev/test only) | Lightweight, standards-compliant MQTT 3.1.1/5.0 broker, official Eclipse project, widely used in IoT |
| **Logging** | log/slog | stdlib (Go 1.23) | Structured logging | Zero dependencies, human-readable text output, leveled logging (DEBUG/INFO/ERROR), context-aware, official Go standard as of 1.21 |
| **Configuration** | os.Getenv | stdlib | Environment variable handling | Zero dependencies, twelve-factor app compliance, Docker-native, educational simplicity |
| **Container Base (Builder)** | golang:alpine | ~1.23-alpine | Multi-stage build (compile stage) | Full Go toolchain, Alpine-based for minimal size |
| **Container Base (Runtime)** | Alpine Linux | ~3.23 | Multi-stage build (runtime stage) | Minimal footprint (~5MB base), security-focused, musl libc compatible with static Go binaries |
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

## Version Notation

- **`~X.Y.Z`** notation means: "version X.Y.Z or any newer patch version (X.Y.*)"
  - Example: `~1.5.0` allows `1.5.1`, `1.5.2`, etc. (patch updates) but not `1.6.0` (minor update)
- **`stdlib`** means: tied to Go version (1.23.x)
- **`N/A (SaaS)`** means: cloud service with automatic updates

## Deployment Architecture

**Primary deployment target:** Self-hosted Proxmox server with Docker
- Images built locally on Proxmox using `docker build`
- Docker Hub used as optional backup/distribution channel
- GitHub Actions CI validates builds but production images built on-site
- Eliminates registry pull dependencies during deployment

---
