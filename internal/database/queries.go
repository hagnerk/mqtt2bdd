package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const maxSensorLen = 255

// InsertMessage inserts a sensor metric row into sensor_metrics.
// If sensor exceeds 255 characters, it is truncated to the last 255 characters
// and a WARN is logged. Uses ON CONFLICT (sensor, date) DO NOTHING for idempotent
// writes. Logs DEBUG on success, WARN on duplicate, ERROR on failure.
func (c *Client) InsertMessage(ctx context.Context, sensor string, timestamp time.Time, metrics json.RawMessage) error {
	sensorRunes := []rune(sensor)
	if len(sensorRunes) > maxSensorLen {
		c.logger.Warn("sensor name truncated",
			"event", "topic_truncated",
			"original_length", len(sensorRunes),
			"truncated_topic", string(sensorRunes[len(sensorRunes)-maxSensorLen:]),
		)
		sensor = string(sensorRunes[len(sensorRunes)-maxSensorLen:])
	}

	const query = `INSERT INTO sensor_metrics (sensor, date, metrics) VALUES ($1, $2, $3) ON CONFLICT (sensor, date) DO NOTHING`

	start := time.Now()
	commandTag, err := c.pool.Exec(ctx, query, sensor, timestamp, metrics)
	elapsed := time.Since(start)
	if err != nil {
		c.connected.Store(false)
		c.logger.Error("failed to insert message",
			"event", "write_failure",
			"operation", "insert",
			"topic", sensor,
			"payload_size", len(metrics),
			"duration_ms", elapsed.Milliseconds(),
			"error", err,
		)
		return fmt.Errorf("failed to insert message for sensor %s: %w", sensor, err)
	}

	// Recorded once, before the duplicate/inserted split: a duplicate is a completed
	// round-trip to a reachable database and a message that left the buffer for good,
	// so it counts as both a successful interaction and a processed message.
	c.connected.Store(true)
	c.lastWriteAt.Store(time.Now().UnixNano())
	c.writeCount.Add(1)

	if commandTag.RowsAffected() == 0 {
		c.logger.Warn("duplicate message ignored",
			"event", "write_duplicate",
			"topic", sensor,
			"timestamp", timestamp.Format(time.RFC3339),
			"payload_size", len(metrics),
			"duration_ms", elapsed.Milliseconds(),
		)
	} else {
		c.logger.Debug("message persisted",
			"event", "write_success",
			"topic", sensor,
			"timestamp", timestamp.Format(time.RFC3339),
			"payload_size", len(metrics),
			"duration_ms", elapsed.Milliseconds(),
			"query", query,
		)
	}

	return nil
}
