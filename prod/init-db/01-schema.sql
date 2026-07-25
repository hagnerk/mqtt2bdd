-- MQTT2BDD production database schema.
-- Executed once by the postgres container, on an empty data directory only.
-- Matches docs/architecture/database-schema.md#table-sensor_metrics.

CREATE TABLE IF NOT EXISTS sensor_metrics (
    id      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sensor  VARCHAR(255) NOT NULL,
    date    TIMESTAMP    NOT NULL,
    metrics JSONB        NOT NULL,
    CONSTRAINT unique_sensor_date UNIQUE (sensor, date)
);

-- Composite index for the time-range queries Grafana issues.
CREATE INDEX IF NOT EXISTS idx_sensor_metrics_sensor_date
    ON sensor_metrics (sensor, date DESC);

-- GIN index so filters on JSON fields inside metrics do not scan the table.
CREATE INDEX IF NOT EXISTS idx_sensor_metrics_metrics
    ON sensor_metrics USING GIN (metrics);
