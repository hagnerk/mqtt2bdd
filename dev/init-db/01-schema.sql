CREATE TABLE IF NOT EXISTS sensor_metrics (
    id     BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sensor VARCHAR(255) NOT NULL,
    date   TIMESTAMP   NOT NULL,
    metrics JSONB      NOT NULL,
    CONSTRAINT unique_sensor_date UNIQUE (sensor, date)
);

CREATE INDEX IF NOT EXISTS idx_sensor_metrics_sensor_date
    ON sensor_metrics (sensor, date DESC);

CREATE INDEX IF NOT EXISTS idx_sensor_metrics_metrics
    ON sensor_metrics USING GIN (metrics);
