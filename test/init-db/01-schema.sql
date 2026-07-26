-- MQTT2BDD test database schema.
-- Executed once per test run, against an ephemeral (tmpfs) data directory.
-- Matches docs/architecture/database-schema.md#table-sensor_metrics, and dev/init-db/01-schema.sql
-- and prod/init-db/01-schema.sql byte-for-byte apart from this comment.

CREATE TABLE IF NOT EXISTS sensor_metrics (
    id      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sensor  VARCHAR(255) NOT NULL,
    date    TIMESTAMP    NOT NULL,
    metrics JSONB        NOT NULL,
    CONSTRAINT unique_sensor_date UNIQUE (sensor, date)
);

CREATE INDEX IF NOT EXISTS idx_sensor_metrics_sensor_date
    ON sensor_metrics (sensor, date DESC);

CREATE INDEX IF NOT EXISTS idx_sensor_metrics_metrics
    ON sensor_metrics USING GIN (metrics);
