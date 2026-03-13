# Coding Standards

This section defines the Go coding standards for MQTT2BDD. These rules are enforced automatically where possible (tooling) and by convention where not. Consistency across the codebase is the primary goal—especially important for an educational project meant to demonstrate idiomatic Go.

## Tooling Enforcement (Non-Negotiable)

All code **must** pass these checks before merging (enforced in CI):

| Tool | Command | What it enforces |
|------|---------|-----------------|
| `go fmt` | `go fmt ./...` | Canonical formatting (indentation, spacing, braces) |
| `go vet` | `go vet ./...` | Common correctness errors (printf mismatches, unreachable code) |
| `staticcheck` | `staticcheck ./...` | Advanced static analysis (deprecated API usage, unused params, etc.) |

**Rule:** No exceptions. If a tool flags something, fix the code—do not suppress the warning unless there is an explicit, documented reason.

## Code Formatting

- **Formatter:** `go fmt` (gofmt) — the official Go formatter, non-negotiable
- **Tab width:** Tabs (not spaces) — enforced by gofmt
- **Line length:** No hard or soft limit. `gofmt` itself does not wrap lines, and modern screens are wide enough that an arbitrary character cap would only force shorter, less descriptive variable names. **Readability takes precedence:** prefer an explicit name like `retryIntervalOnDatabaseFailure` on a long line over an abbreviated `retryDbInt` that fits within an arbitrary column count.
- **Braces:** Opening brace on the same line (enforced by gofmt):
  ```go
  // Correct
  func connect() error {
      ...
  }

  // Incorrect (gofmt will fix this)
  func connect() error
  {
      ...
  }
  ```

## Naming Conventions

Following the [official Go naming guide](https://go.dev/doc/effective_go#names):

**Packages:**
- Lowercase, single word, no underscores: `config`, `mqtt`, `database`, `logger`
- Package name = last element of import path
- Avoid generic names like `util`, `common`, `helpers`

```go
// Correct
package config
package mqtt
package database

// Incorrect
package Config
package mqtt_client
package utils
```

**Variables and Functions:**
- `camelCase` for unexported: `bufferSize`, `retryInterval`, `msgChan`
- `PascalCase` for exported: `LoadConfig()`, `NewClient()`, `InsertMessage()`
- Short, contextual names in small scopes: `i`, `n`, `err`, `msg`
- Descriptive names in larger scopes: `mqttClient`, `dbClient`, `shutdownCtx`

```go
// Good - short names in short scopes
for i, msg := range messages {
    ...
}

// Good - descriptive names in function signatures
func NewClient(cfg *Config, logger *slog.Logger) *Client {
    ...
}
```

**Constants:**
- `PascalCase` for exported, `camelCase` for unexported:
  ```go
  const DefaultBufferSize = 1000                     // exported
  const defaultRetryInterval = 10 * time.Second      // unexported
  ```

**Interfaces:**
- Single-method interfaces: name = method + `er`: `Connecter`, `Inserter`
- Prefer small, focused interfaces (Go proverb: "The bigger the interface, the weaker the abstraction")

**Error variables:**
- Prefix with `Err`: `ErrConnectionFailed`, `ErrInvalidConfig`
  ```go
  var ErrMissingEnvVar = errors.New("required environment variable not set")
  ```

**Structs:**
- `PascalCase` for exported: `Config`, `Client`, `Message`
- Field names: `PascalCase` for exported, `camelCase` for unexported

## Import Organization

Imports are grouped in three blocks, separated by blank lines:

```go
import (
    // 1. Standard library
    "context"
    "fmt"
    "os"
    "time"

    // 2. External dependencies
    mqtt "github.com/eclipse/paho.mqtt.golang"
    "github.com/jackc/pgx/v5/pgxpool"

    // 3. Internal packages (same module)
    "github.com/username/mqtt2bdd/internal/config"
    "github.com/username/mqtt2bdd/internal/logger"
)
```

> **Note on internal import paths:** The prefix `github.com/username/mqtt2bdd` is the **module name** declared in `go.mod` — it is not a network URL. When building locally, the Go toolchain resolves all imports whose prefix matches the current module to local files on disk. It never fetches from GitHub. This means that if you are editing `internal/config/config.go` locally, any file importing `github.com/username/mqtt2bdd/internal/config` will automatically use your local, in-progress version. This is standard Go module behaviour.

`goimports` (or `gopls` in VS Code) handles grouping and ordering automatically.

## Error Handling

See **Error Handling Strategy** section for the full strategy. Key coding conventions:

1. **Always check errors — never discard with `_`** (except in tests where the result is irrelevant):
   ```go
   // Correct
   if err := client.Connect(ctx); err != nil {
       return fmt.Errorf("connect failed: %w", err)
   }

   // Incorrect
   client.Connect(ctx)  // silently ignores error
   ```

2. **Wrap errors with context using `%w`:**
   ```go
   return fmt.Errorf("failed to insert message for sensor %s: %w", sensor, err)
   ```

3. **Return early on error (avoid deep nesting):**
   ```go
   // Correct - early return
   result, err := doSomething()
   if err != nil {
       return err
   }
   // use result

   // Incorrect - pyramid of doom
   result, err := doSomething()
   if err == nil {
       // 10+ lines of code
       if anotherErr == nil {
           // more code
       }
   }
   ```

4. **No `panic()` in production code** — only allowed in `init()` or `main()` for programmer errors that indicate a broken build, never for runtime conditions.

## Functions and Methods

- **Single responsibility:** Each function does one thing
- **Short functions preferred:** If a function exceeds ~40 lines, consider splitting
- **Constructor pattern:** `NewXxx(...)` returns `*Xxx` and an error when initialization can fail:
  ```go
  func NewClient(cfg *config.Config, logger *slog.Logger) (*Client, error) {
      ...
  }
  ```
- **Receiver naming:** Short, lowercase abbreviation of type name (consistent across all methods):
  ```go
  func (c *Client) Connect(ctx context.Context) error { ... }
  func (c *Client) Disconnect() { ... }
  // 'c' used consistently, not 'client' or 'cl'
  ```
- **Value vs pointer receivers:** Use pointer receivers (`*T`) for all methods on structs that hold state (connection pools, loggers). Be consistent — if any method uses a pointer receiver, all should.

## Comments and Documentation

Following [godoc conventions](https://pkg.go.dev/golang.org/x/tools/cmd/godoc):

- **Exported symbols must have doc comments:**
  ```go
  // LoadConfig reads application configuration from environment variables.
  // Returns an error if any required variable is missing or invalid.
  func LoadConfig() (*Config, error) {
  ```

- **Package comment on the first file of the package:**
  ```go
  // Package config handles loading and validating application configuration
  // from environment variables following twelve-factor app principles.
  package config
  ```

- **Comment style:** Full sentences starting with the symbol name, ending with a period
- **No obvious comments** — explain *why*, not *what*:
  ```go
  // Correct - explains why
  // BIGINT handles 25,000+ years of inserts at 1 million messages/day without overflow.
  id BIGINT GENERATED ALWAYS AS IDENTITY

  // Incorrect - just restates the code
  // Set id column
  id BIGINT
  ```

- **TODOs:** `// TODO(username): description` format for tracked future work

## Concurrency

- **Share memory by communicating** (Go proverb) — use channels, not shared variables + mutexes where possible
- **Document goroutine ownership:** Comment on which goroutine owns each channel end
- **Always pair `go` with a completion mechanism** (`sync.WaitGroup`, channel, or context):
  ```go
  var wg sync.WaitGroup
  wg.Add(1)
  go func() {
      defer wg.Done()
      dbWriterLoop(msgChan, dbClient, logger)
  }()
  ```
- **Avoid goroutine leaks:** Every goroutine must have a clear exit condition (channel close, context cancellation, or signal)
- **Context propagation:** Pass `context.Context` as the first argument to any function that does I/O:
  ```go
  func (c *Client) InsertMessage(ctx context.Context, sensor string, ts time.Time, metrics json.RawMessage) error
  ```

## Constants and Configuration

- **No magic numbers** — use named constants or config values:
  ```go
  // Correct
  const defaultBufferSize = 1000
  const defaultRetryInterval = 10 * time.Second

  // Incorrect
  msgChan := make(chan Message, 1000)
  time.Sleep(10 * time.Second)
  ```

- **Configuration via environment only** — no hardcoded hosts, ports, or credentials anywhere in the code

## File Organization

Each Go source file follows this structure (top to bottom):

1. Package declaration + package comment
2. Import block
3. Constants (`const` block)
4. Package-level variables (`var` block) — minimize; prefer local variables
5. Type declarations (`type` block)
6. Constructor functions (`NewXxx`)
7. Methods (grouped by receiver type)
8. Unexported helper functions

## Summary of Non-Negotiables

| Rule | Enforced by |
|------|------------|
| Code formatted with `go fmt` | CI pipeline |
| No `go vet` warnings | CI pipeline |
| No `staticcheck` warnings | CI pipeline |
| All errors checked (no `_` on errors) | Code review |
| No `panic()` in non-init code | Code review |
| Exported symbols have doc comments | Code review |
| No hardcoded credentials or hostnames | Code review + `.gitignore` |

---
