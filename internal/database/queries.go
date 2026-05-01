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
			"original_length", len(sensorRunes),
			"truncated_sensor", string(sensorRunes[len(sensorRunes)-maxSensorLen:]),
		)
		sensor = string(sensorRunes[len(sensorRunes)-maxSensorLen:])
	}

	const query = `INSERT INTO sensor_metrics (sensor, date, metrics) VALUES ($1, $2, $3) ON CONFLICT (sensor, date) DO NOTHING`

	commandTag, err := c.pool.Exec(ctx, query, sensor, timestamp, metrics)
	if err != nil {
		c.logger.Error("failed to insert message",
			"sensor", sensor,
			"error", err,
		)
		return fmt.Errorf("failed to insert message for sensor %s: %w", sensor, err)
	}

	if commandTag.RowsAffected() == 0 {
		c.logger.Warn("duplicate message ignored",
			"sensor", sensor,
			"timestamp", timestamp.Format(time.RFC3339),
		)
	} else {
		c.logger.Debug("message persisted",
			"sensor", sensor,
			"timestamp", timestamp.Format(time.RFC3339),
			"payload_size", len(metrics),
		)
	}

	return nil
}
