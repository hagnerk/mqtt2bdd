package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const maxSensorLen = 255

// ErrRejected marks a message that must not be retried: its own content can never be
// stored, so retrying it would only hold back every message behind it. Callers match it
// with errors.Is.
var ErrRejected = errors.New("message rejected by database")

// InsertMessage inserts a sensor metric row into sensor_metrics.
// If sensor exceeds 255 characters, it is truncated to the last 255 characters
// and a WARN is logged. Uses ON CONFLICT (sensor, date) DO NOTHING for idempotent
// writes. Logs DEBUG on success, WARN on duplicate, ERROR on failure.
//
// An empty payload (MQTT's way of clearing a retained message) is skipped without a
// database round-trip and returns nil. A payload that is not valid JSON is rejected
// locally, and one PostgreSQL refuses for its content (SQLSTATE class 22, 23 or 54) is
// rejected after the round-trip: both return an error wrapping ErrRejected, which callers
// must not retry. Every other error is transient and worth retrying.
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

	// Step 1: nothing to store. A nil slice has length zero and lands here too.
	if len(metrics) == 0 {
		c.logger.Debug("empty payload skipped", "event", "write_skipped", "topic", sensor, "reason", "empty_payload")
		return nil
	}

	// Step 2: a payload that is not JSON can never be stored, so the database is not asked.
	if !json.Valid(metrics) {
		rejectErr := fmt.Errorf("%w: invalid JSON", ErrRejected)
		c.recordRejection(sensor, len(metrics), 0, rejectErr, "reason", "invalid_json")
		return rejectErr
	}

	const query = `INSERT INTO sensor_metrics (sensor, date, metrics) VALUES ($1, $2, $3) ON CONFLICT (sensor, date) DO NOTHING`

	start := time.Now()
	commandTag, err := c.pool.Exec(ctx, query, sensor, timestamp, metrics)
	elapsed := time.Since(start)
	if err != nil && isPermanent(err) {
		// The database answered, so it is reachable; nothing was written, so the write
		// fields stay as they are.
		c.connected.Store(true)
		c.recordRejection(sensor, len(metrics), elapsed, err, "reason", "sqlstate", "sqlstate", sqlState(err))
		return fmt.Errorf("failed to insert message for sensor %s: %w: %w", sensor, ErrRejected, err)
	}
	if err != nil {
		c.connected.Store(false)
		c.logger.Error("failed to insert message",
			"event", "write_failure",
			"operation", "insert",
			"topic", sensor,
			"payload_size", len(metrics),
			"duration_us", elapsed.Microseconds(),
			"error", err,
		)
		return fmt.Errorf("failed to insert message for sensor %s: %w", sensor, err)
	}

	c.recordWrite(sensor, timestamp, len(metrics), elapsed, commandTag.RowsAffected() == 0, query)
	return nil
}

// recordWrite updates the health fields after a completed INSERT and logs its outcome.
func (c *Client) recordWrite(sensor string, timestamp time.Time, payloadSize int, elapsed time.Duration, duplicate bool, query string) {
	// Recorded once, before the duplicate/inserted split: a duplicate is a completed
	// round-trip to a reachable database and a message that left the buffer for good,
	// so it counts as both a successful interaction and a processed message.
	c.connected.Store(true)
	c.lastWriteAt.Store(time.Now().UnixNano())
	c.writeCount.Add(1)

	if duplicate {
		c.logger.Warn("duplicate message ignored",
			"event", "write_duplicate",
			"topic", sensor,
			"timestamp", timestamp.Format(time.RFC3339),
			"payload_size", payloadSize,
			"duration_us", elapsed.Microseconds(),
		)
	} else {
		c.logger.Debug("message persisted",
			"event", "write_success",
			"topic", sensor,
			"timestamp", timestamp.Format(time.RFC3339),
			"payload_size", payloadSize,
			"duration_us", elapsed.Microseconds(),
			"query", query,
		)
	}
}

// recordRejection counts one discarded message and logs it once, at ERROR: a discarded
// message is lost data. reasonAttrs carries reason and, for a server rejection, sqlstate.
// The payload body is never logged: it can be hundreds of kilobytes, and the DEBUG
// message_received entry already carries it.
func (c *Client) recordRejection(sensor string, payloadSize int, elapsed time.Duration, err error, reasonAttrs ...any) {
	c.rejectedCount.Add(1)
	attrs := []any{
		"event", "write_rejected",
		"operation", "insert",
		"topic", sensor,
		"payload_size", payloadSize,
		"duration_us", elapsed.Microseconds(),
	}
	attrs = append(attrs, reasonAttrs...)
	attrs = append(attrs, "error", err)
	c.logger.Error("message rejected", attrs...)
}

// isPermanent reports whether err is PostgreSQL refusing the message itself, so that
// retrying it can never succeed. The list is closed on purpose: classifying a transient
// error as permanent loses a message, the opposite only delays the writer, so anything
// not known to be about the message's content is transient. Class 42 (syntax error or
// access rule violation) stays transient although it never fixes itself: it hits every
// message identically and an operator can fix it (a missing GRANT, a missing table), so
// waiting keeps the buffered messages instead of discarding all of them.
func isPermanent(err error) bool {
	code := sqlState(err)
	// PostgreSQL always sends five characters, but a synthetic PgError may carry fewer.
	if len(code) < 2 {
		return false
	}
	switch code[:2] {
	case "22", "23", "54": // data exception, integrity constraint violation, program limit exceeded
		return true
	}
	return false
}

// sqlState returns the SQLSTATE code of the PostgreSQL error wrapped in err, or "" when
// err carries none.
func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return ""
	}
	return pgErr.Code
}
