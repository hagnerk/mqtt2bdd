//go:build integration

package main

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"
)

// TestPayloadRepair proves that a message PostgreSQL refuses for repairable content is
// stored instead of discarded. Before Story 5.2 each message below was rejected
// (SQLSTATE 22P05, 22P02 and 22021) and never stored; now each one is stored with U+FFFD
// in place of its defect, and the repair is logged once at WARN.
func TestPayloadRepair(t *testing.T) {
	env := map[string]string{
		"MQTT_BROKER":       testMQTTBroker,
		"MQTT_PORT":         testMQTTPort,
		"POSTGRES_HOST":     testPostgresHost,
		"POSTGRES_PORT":     testPostgresPort,
		"POSTGRES_DB":       testPostgresDB,
		"POSTGRES_USER":     testPostgresUser,
		"POSTGRES_PASSWORD": testPostgresPass,
	}
	app := startTestApp(t, env)
	app.waitForEvent(t, "startup_complete", 30*time.Second)

	pool := openTestDBPool(t)

	suffix := time.Now().UnixNano()
	cases := []struct {
		topic   string
		payload string
		counts  string
	}{
		// JSON escapes in raw strings: in an interpreted string \u0000 would be a NUL byte.
		{fmt.Sprintf("integration/repair/nul-escape-%d", suffix), `{"a":"x\u0000y"}`, "nul_escapes=1 lone_surrogates=0 invalid_utf8_sequences=0"},
		{fmt.Sprintf("integration/repair/lone-surrogate-%d", suffix), `{"a":"x\ud83dy"}`, "nul_escapes=0 lone_surrogates=1 invalid_utf8_sequences=0"},
		{fmt.Sprintf("integration/repair/invalid-utf8-%d", suffix), "{\"a\":\"x\xffy\"}", "nul_escapes=0 lone_surrogates=0 invalid_utf8_sequences=1"},
	}

	// QoS 0 and never retained: a retained message would be redelivered to every later test.
	for _, c := range cases {
		publishTestMessage(t, c.topic, c.payload)
	}

	for _, c := range cases {
		// A stall here means the message was rejected or retried instead of repaired.
		if !waitForRow(pool, c.topic, defaultRetryIntervalOnDatabaseFailure/2) {
			t.Fatalf("message on %s was not persisted within %s", c.topic, defaultRetryIntervalOnDatabaseFailure/2)
		}
		var got string
		err := pool.QueryRow(context.Background(),
			"SELECT metrics->>'a' FROM sensor_metrics WHERE sensor = $1", c.topic).Scan(&got)
		if err != nil {
			t.Fatalf("verification query for %s failed: %v", c.topic, err)
		}
		if got != "x\uFFFDy" {
			t.Errorf("stored value for %s = %q, want %q", c.topic, got, "x\uFFFDy")
		}
	}

	sanitized := waitForEventCount(t, app, "payload_sanitized", len(cases))
	payloadRe := regexp.MustCompile(`\spayload=`)
	for _, c := range cases {
		topicRe := regexp.MustCompile(`topic=` + regexp.QuoteMeta(c.topic) + `(\s|$)`)
		var matched []string
		for _, line := range sanitized {
			if topicRe.MatchString(line) {
				matched = append(matched, line)
			}
		}
		if len(matched) != 1 {
			t.Errorf("expected exactly one payload_sanitized entry for %s, got %d:\n%v", c.topic, len(matched), sanitized)
			continue
		}
		line := matched[0]
		if !regexp.MustCompile(`(^|\s)level=WARN\s`).MatchString(line) {
			t.Errorf("payload_sanitized entry must be WARN: %q", line)
		}
		if !regexp.MustCompile(`\s` + regexp.QuoteMeta(c.counts) + `(\s|$)`).MatchString(line) {
			t.Errorf("payload_sanitized entry for %s must carry %q: %q", c.topic, c.counts, line)
		}
		if regexp.MustCompile(`\soperation=`).MatchString(line) {
			t.Errorf("payload_sanitized reports a state and must carry no operation: %q", line)
		}
		if payloadRe.MatchString(line) {
			t.Errorf("payload_sanitized entry must never carry the payload body: %q", line)
		}
		for _, rejected := range app.eventLines("write_rejected") {
			if topicRe.MatchString(rejected) {
				t.Errorf("a repaired message must not be rejected, got: %q", rejected)
			}
		}
	}

	app.terminate(t)
	app.waitForExit(t, defaultShutdownTimeout+5*time.Second)
}

// waitForEventCount polls the captured stdout until exactly want entries of event are
// present, and returns them. It fails the test if the count is not reached within
// logCaptureTimeout, or is exceeded.
func waitForEventCount(t *testing.T, app *testApp, event string, want int) []string {
	t.Helper()
	deadline := time.Now().Add(logCaptureTimeout)
	for {
		lines := app.eventLines(event)
		if len(lines) == want {
			return lines
		}
		if len(lines) > want || time.Now().After(deadline) {
			t.Fatalf("expected %d %s entries, got %d.\nCaptured lines:\n%v", want, event, len(lines), app.allLines())
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}
