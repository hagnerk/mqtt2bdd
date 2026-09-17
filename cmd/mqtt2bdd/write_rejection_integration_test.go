//go:build integration

package main

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"
)

// logCaptureTimeout bounds how long the test waits for log lines to reach the capture
// buffer: stdout is drained by a separate goroutine, so a line written before the valid
// INSERT may not have been captured yet when its row appears.
const logCaptureTimeout = 5 * time.Second

// TestWriteRejection proves that a message the database can never store no longer
// blocks the writer. Before Story 5.1 the first message below was retried forever, so
// the valid message published last was never stored. Now the non-JSON and the
// server-rejected payloads are discarded and logged, the empty payload is skipped, and
// the valid payload is stored well within one retry interval.
func TestWriteRejection(t *testing.T) {
	env := map[string]string{
		"MQTT_BROKER":       testMQTTBroker,
		"MQTT_PORT":         testMQTTPort,
		"POSTGRES_HOST":     testPostgresHost,
		"POSTGRES_PORT":     testPostgresPort,
		"POSTGRES_DB":       testPostgresDB,
		"POSTGRES_USER":     testPostgresUser,
		"POSTGRES_PASSWORD": testPostgresPass,
		// write_skipped is a DEBUG entry.
		"LOG_LEVEL": "DEBUG",
	}
	app := startTestApp(t, env)
	app.waitForEvent(t, "startup_complete", 30*time.Second)

	pool := openTestDBPool(t)

	suffix := time.Now().UnixNano()
	invalidTopic := fmt.Sprintf("integration/rejection/invalid-json-%d", suffix)
	overflowTopic := fmt.Sprintf("integration/rejection/numeric-overflow-%d", suffix)
	emptyTopic := fmt.Sprintf("integration/rejection/empty-%d", suffix)
	validTopic := fmt.Sprintf("integration/rejection/valid-%d", suffix)

	// Published in this order, QoS 0 and never retained: a retained message would be
	// redelivered to every later test. {"v":1e1000000} passes json.Valid but overflows
	// PostgreSQL's numeric type (SQLSTATE 22003).
	start := time.Now()
	publishTestMessage(t, invalidTopic, `online`)
	publishTestMessage(t, overflowTopic, `{"v":1e1000000}`)
	publishTestMessage(t, emptyTopic, ``)
	publishTestMessage(t, validTopic, `{"value": 1}`)

	if !waitForRow(pool, validTopic, defaultRetryIntervalOnDatabaseFailure/2) {
		t.Fatalf("valid message was not persisted within %s: the writer is stuck behind a rejected message", defaultRetryIntervalOnDatabaseFailure/2)
	}
	if elapsed := time.Since(start); elapsed >= defaultRetryIntervalOnDatabaseFailure {
		t.Fatalf("valid message persisted %s after the first publish, want under the %s retry interval", elapsed, defaultRetryIntervalOnDatabaseFailure)
	}

	var discarded int
	err := pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM sensor_metrics WHERE sensor = ANY($1)",
		[]string{invalidTopic, overflowTopic, emptyTopic}).Scan(&discarded)
	if err != nil {
		t.Fatalf("verification query failed: %v", err)
	}
	if discarded != 0 {
		t.Fatalf("expected no row for the rejected and skipped messages, got %d", discarded)
	}

	rejected, skipped := waitForWriteOutcomes(t, app, 2, 1)

	invalidRe := regexp.MustCompile(`topic=` + regexp.QuoteMeta(invalidTopic) + `(\s|$)`)
	overflowRe := regexp.MustCompile(`topic=` + regexp.QuoteMeta(overflowTopic) + `(\s|$)`)
	emptyRe := regexp.MustCompile(`topic=` + regexp.QuoteMeta(emptyTopic) + `(\s|$)`)
	payloadRe := regexp.MustCompile(`\spayload=`)

	var sawInvalid, sawOverflow bool
	for _, line := range rejected {
		if payloadRe.MatchString(line) {
			t.Errorf("write_rejected entry must never carry the payload body: %q", line)
		}
		switch {
		case invalidRe.MatchString(line):
			sawInvalid = regexp.MustCompile(`\sreason=invalid_json(\s|$)`).MatchString(line)
		case overflowRe.MatchString(line):
			sawOverflow = regexp.MustCompile(`\sreason=sqlstate\s`).MatchString(line) &&
				regexp.MustCompile(`\ssqlstate=22003(\s|$)`).MatchString(line)
		}
	}
	if !sawInvalid {
		t.Errorf("expected a write_rejected entry with reason=invalid_json for %s, got:\n%v", invalidTopic, rejected)
	}
	if !sawOverflow {
		t.Errorf("expected a write_rejected entry with reason=sqlstate sqlstate=22003 for %s, got:\n%v", overflowTopic, rejected)
	}

	if !emptyRe.MatchString(skipped[0]) || !regexp.MustCompile(`\sreason=empty_payload(\s|$)`).MatchString(skipped[0]) {
		t.Errorf("expected a write_skipped entry with reason=empty_payload for %s, got %q", emptyTopic, skipped[0])
	}

	if app.hasEvent("write_failure") {
		t.Errorf("a rejection must not be logged as write_failure, got:\n%v", app.eventLines("write_failure"))
	}

	app.terminate(t)
	app.waitForExit(t, defaultShutdownTimeout+5*time.Second)
}

// waitForWriteOutcomes polls the captured stdout until exactly wantRejected write_rejected
// and wantSkipped write_skipped entries are present, and returns them. It fails the test
// if the counts are not reached within logCaptureTimeout, or are exceeded.
func waitForWriteOutcomes(t *testing.T, app *testApp, wantRejected, wantSkipped int) (rejected, skipped []string) {
	t.Helper()
	deadline := time.Now().Add(logCaptureTimeout)
	for {
		rejected = app.eventLines("write_rejected")
		skipped = app.eventLines("write_skipped")
		if len(rejected) > wantRejected || len(skipped) > wantSkipped {
			break
		}
		if len(rejected) == wantRejected && len(skipped) == wantSkipped {
			return rejected, skipped
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected %d write_rejected and %d write_skipped entries, got %d and %d.\nCaptured lines:\n%v",
		wantRejected, wantSkipped, len(rejected), len(skipped), app.allLines())
	return nil, nil
}
