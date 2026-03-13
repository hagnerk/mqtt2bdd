# Database Schema

Transform the conceptual data model into concrete PostgreSQL schema with DDL statements.

## Database: PostgreSQL ~15.10

**Schema Name:** `public` (default schema)

**Database Name:** Configurable via environment variable `POSTGRES_DB` (example: `mqtt2bdd`)

## PostgreSQL vs TimescaleDB Decision

**Evaluated Options:**

**Option A: PostgreSQL Standard (Chosen)**
- Native time-series capability (TIMESTAMP + indexes)
- Zero external dependencies
- Simpler operations and maintenance
- Proven for volumes < 1 billion rows
- Better for learning objectives

**Option B: TimescaleDB Extension**
- Automatic time-based partitioning (hypertables)
- Native compression (90%+ space savings)
- Continuous aggregates (auto-refreshing materialized views)
- Optimized for extreme scale (> 1 billion rows)
- Additional operational complexity

**Decision: PostgreSQL Standard**

**Rationale:**
- **Projected volume:** ~50M rows/year (100 devices × 1 msg/min) = well within PostgreSQL capacity
- **Query patterns:** Simple time-range SELECTs (Grafana) - no complex time-series analytics
- **Educational goal:** Minimize external dependencies, focus on Go learning
- **Operational simplicity:** No extension management, no version compatibility issues

**Migration path:** If volume exceeds 500M rows or storage becomes constrained, migrate to TimescaleDB (schema changes minimal)

## Table: sensor_metrics

**DDL Statement:**

```sql
-- Create table with IDENTITY primary key and composite unique constraint
CREATE TABLE sensor_metrics (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sensor VARCHAR(255) NOT NULL,
    date TIMESTAMP NOT NULL,
    metrics JSONB NOT NULL,
    CONSTRAINT unique_sensor_date UNIQUE (sensor, date)
);

-- Create composite index for typical Grafana queries (sensor + date range)
CREATE INDEX idx_sensor_metrics_sensor_date ON sensor_metrics (sensor, date DESC);

-- Create GIN index for JSONB queries (filtering on JSON fields)
CREATE INDEX idx_sensor_metrics_metrics ON sensor_metrics USING GIN (metrics);
```

**Column Details:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | BIGINT | NOT NULL | IDENTITY | Auto-incrementing unique identifier (SQL:2003 standard), managed exclusively by PostgreSQL |
| `sensor` | VARCHAR(255) | NOT NULL | - | MQTT topic path serving as sensor identifier (e.g., "zigbee2mqtt/bedroom/temperature") |
| `date` | TIMESTAMP | NOT NULL | - | Message timestamp in UTC with microsecond precision for time-series ordering |
| `metrics` | JSONB | NOT NULL | - | Raw JSON payload containing sensor readings in binary format for efficient querying |

**Indexes:**

1. **Primary Key Index (Automatic):** B-tree index on `id` column
2. **Composite Index:** `idx_sensor_metrics_sensor_date` - B-tree index on `(sensor, date DESC)`
3. **GIN Index:** `idx_sensor_metrics_metrics` - Generalized Inverted Index on `metrics` JSONB column
   - **Trade-off:** Index ~30% of data size, INSERTs ~10% slower, JSON queries 10-100x faster

## Schema Initialization

**Location:** `dev/init-db/01-schema.sql`

```sql
-- MQTT2BDD Database Schema
-- Version: 1.0
-- Purpose: Time-series storage for MQTT sensor messages

-- Create sensor_metrics table
CREATE TABLE IF NOT EXISTS sensor_metrics (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sensor VARCHAR(255) NOT NULL,
    date TIMESTAMP NOT NULL,
    metrics JSONB NOT NULL,
    CONSTRAINT unique_sensor_date UNIQUE (sensor, date)
);

-- Create composite index for time-series queries
CREATE INDEX IF NOT EXISTS idx_sensor_metrics_sensor_date
ON sensor_metrics (sensor, date DESC);

-- Create GIN index for JSONB queries
CREATE INDEX IF NOT EXISTS idx_sensor_metrics_metrics
ON sensor_metrics USING GIN (metrics);

-- Display table info
\d sensor_metrics

-- Display indexes
\di sensor_metrics*
```

## Application INSERT Operations

**Standard Insert (Go code with duplicate detection):**

```go
func (c *Client) InsertMessage(ctx context.Context, sensor string, timestamp time.Time, metrics json.RawMessage) error {
    query := `
        INSERT INTO sensor_metrics (sensor, date, metrics)
        VALUES ($1, $2, $3)
        ON CONFLICT (sensor, date) DO NOTHING
    `

    // Use Exec which returns CommandTag with rows affected
    commandTag, err := c.pool.Exec(ctx, query, sensor, timestamp, metrics)
    if err != nil {
        return fmt.Errorf("failed to insert message: %w", err)
    }

    // Check if row was actually inserted (0 = conflict occurred)
    rowsAffected := commandTag.RowsAffected()
    if rowsAffected == 0 {
        c.logger.Warn("Duplicate message ignored (conflict on sensor+date)",
            "sensor", sensor,
            "timestamp", timestamp.Format(time.RFC3339),
        )
    } else {
        c.logger.Debug("Message persisted",
            "sensor", sensor,
            "timestamp", timestamp.Format(time.RFC3339),
            "payload_size", len(metrics),
        )
    }

    return nil
}
```

**Key Features:**
- **Parameterized query:** Prevents SQL injection
- **ON CONFLICT DO NOTHING:** Idempotent writes
- **RowsAffected() check:** Detects duplicates (0 = conflict, 1 = inserted)
- **Conditional logging:** DEBUG for success, WARN for duplicates

## Storage Estimates

| Metric | Value | Calculation |
|--------|-------|-------------|
| Messages/day | 144,000 | 100 devices × 60 min × 24 hours |
| Messages/year | 52,560,000 | 144,000 × 365 |
| Row size | ~300 bytes | 8 (id) + ~30 (sensor) + 8 (date) + 200 (metrics) + overhead |
| Storage/year | ~15 GB | 52.5M rows × 300 bytes |
| Index size (B-tree) | ~3 GB | Composite index |
| Index size (GIN) | ~5 GB | GIN index on JSONB |
| **Total/year** | **~23 GB** | Data + indexes |

**5-Year Projection:** ~115 GB

## Example Queries (Grafana)

**Query 1: Latest 1000 readings**
```sql
SELECT date, metrics
FROM sensor_metrics
WHERE sensor = 'zigbee2mqtt/bedroom/thermostat'
ORDER BY date DESC
LIMIT 1000;
```

**Query 2: Temperature trend (24 hours)**
```sql
SELECT
    date,
    (metrics->>'temperature')::numeric AS temperature
FROM sensor_metrics
WHERE sensor = 'zigbee2mqtt/bedroom/thermostat'
  AND date > NOW() - INTERVAL '24 hours'
ORDER BY date ASC;
```

**Query 3: Hourly average**
```sql
SELECT
    date_trunc('hour', date) AS hour,
    AVG((metrics->>'temperature')::numeric) AS avg_temp
FROM sensor_metrics
WHERE sensor = 'zigbee2mqtt/bedroom/thermostat'
  AND date > NOW() - INTERVAL '7 days'
GROUP BY hour
ORDER BY hour ASC;
```

## Index Maintenance

**VACUUM ANALYZE (Updates statistics):**
- **Frequency:** Monthly (optional - autovacuum handles most cases)

```bash
# Monthly cron job (first day of month at 3 AM)
0 3 1 * * docker exec mqtt2bdd-prod-postgres psql -U user -d mqtt2bdd -c "VACUUM ANALYZE sensor_metrics;"
```

**REINDEX (Rebuilds indexes):**
- **Frequency:** Quarterly (optional for INSERT-only tables)

```bash
# Quarterly cron job (first day of quarter at 3 AM)
0 3 1 */3 * docker exec mqtt2bdd-prod-postgres psql -U user -d mqtt2bdd -c "REINDEX TABLE CONCURRENTLY sensor_metrics;"
```

**Note:** With `autovacuum = on` (default), manual maintenance is largely optional.

## Performance Tuning

**Recommended `postgresql.conf` settings:**

```ini
# Memory settings (for 8GB RAM server)
shared_buffers = 2GB
effective_cache_size = 6GB
work_mem = 64MB

# Write-heavy optimizations
wal_buffers = 16MB
checkpoint_completion_target = 0.9
max_wal_size = 2GB

# Autovacuum tuning
autovacuum = on
autovacuum_max_workers = 2
autovacuum_naptime = 30s
```

---
