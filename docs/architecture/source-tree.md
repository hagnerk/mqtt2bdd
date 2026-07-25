# Source Tree

Project folder structure reflecting the monorepo organization, Go project layout best practices, and containerized development/deployment environments.

## Complete Project Structure

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
├── prod/
│   ├── init-db/
│   │   └── 01-schema.sql                   # Production database schema init
│   └── mosquitto/
│       └── mosquitto.conf                  # Production Mosquitto config
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
├── .env.prod.example                       # Production env template (no real values)
├── Dockerfile                              # Production multi-stage build
├── docker-compose.prod.yml                 # Production deployment stack
├── go.mod                                  # Go module definition
├── go.sum                                  # Go module checksums
├── Makefile                                # Common tasks automation
├── README.md                               # Project overview
└── LICENSE                                 # License file
```

## Package Import Paths

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

## Design Rationale

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
