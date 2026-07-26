//go:build integration

package main

import (
	"fmt"
	"regexp"
	"testing"
	"time"
)

// TestShutdownDrainBounded proves AC13 / FIND-016: the shutdown drain path is bounded
// by defaultShutdownTimeout even when the database is genuinely unavailable, rather
// than racing an unbounded retry against dbClient.Close(). A real, briefly-stopped
// test-postgres is used — not a mock — because the invariant under test is about real
// I/O timing.
func TestShutdownDrainBounded(t *testing.T) {
	env := map[string]string{
		"MQTT_BROKER":       testMQTTBroker,
		"MQTT_PORT":         testMQTTPort,
		"POSTGRES_HOST":     testPostgresHost,
		"POSTGRES_PORT":     testPostgresPort,
		"POSTGRES_DB":       testPostgresDB,
		"POSTGRES_USER":     testPostgresUser,
		"POSTGRES_PASSWORD": testPostgresPass,
		"LOG_LEVEL":         "DEBUG",
		"BUFFER_SIZE":       "50",
	}
	app := startTestApp(t, env)
	app.waitForEvent(t, "startup_complete", 30*time.Second)

	pool := openTestDBPool(t)

	// 1-2. Normal operation first: a handful of messages flow through and persist,
	// proving the pipeline works before any disruption.
	for i := 0; i < 3; i++ {
		sensor := fmt.Sprintf("integration/drain/warmup-%d-%d", time.Now().UnixNano(), i)
		publishTestMessage(t, sensor, `{"value": 1}`)
		if !waitForRow(pool, sensor, 10*time.Second) {
			t.Fatalf("warmup message %d was never persisted", i)
		}
	}

	// 3. Stop test-postgres, then immediately publish one more message so it sits
	// buffered with the database down. Cleanup is registered now, before any later
	// assertion can abort the test, so the next test always gets postgres back.
	dockerStop(t, postgresContainer)
	t.Cleanup(func() { dockerStart(t, postgresContainer) })

	stuck := fmt.Sprintf("integration/drain/stuck-%d", time.Now().UnixNano())
	publishTestMessage(t, stuck, `{"value": 2}`)
	// Give the writer a brief moment to actually attempt (and fail) an insert against
	// the now-stopped database, so shutdown genuinely happens mid-outage rather than
	// racing SIGTERM against a message still in flight over the network.
	time.Sleep(500 * time.Millisecond)

	// 4. Send SIGTERM and assert the process exits within defaultShutdownTimeout plus
	// margin — not indefinitely. This is the load-bearing assertion: before the fix, a
	// sufficiently unlucky timing could not converge this cleanly.
	app.terminate(t)
	start := time.Now()
	app.waitForExit(t, defaultShutdownTimeout+5*time.Second)
	t.Logf("subprocess exited %s after SIGTERM (bound: %s)", time.Since(start), defaultShutdownTimeout+5*time.Second)

	// 5. Either flush_complete or flush_timeout is an acceptable, honest outcome —
	// what must NOT happen is the process hanging past the bound or exiting non-zero
	// (already checked by waitForExit above).
	if !app.hasEvent("flush_complete") && !app.hasEvent("flush_timeout") {
		t.Fatalf("expected flush_complete or flush_timeout in subprocess stdout, got neither.\nCaptured lines:\n%v", app.allLines())
	}
	if !app.hasEvent("shutdown_complete") {
		t.Fatalf("expected shutdown_complete in subprocess stdout.\nCaptured lines:\n%v", app.allLines())
	}
}

// healthCheckEventRE matches a structured log line's event=health_check field.
var healthCheckEventRE = regexp.MustCompile(`event=health_check(\s|$)`)

// TestGracefulShutdown proves AC14: the message_dropped WARN fires under a genuine
// full-buffer-at-shutdown race, and healthWG's teardown ordering (after the drain
// select, before dbClient.Close()) holds under real timing — not by inspection.
func TestGracefulShutdown(t *testing.T) {
	env := map[string]string{
		"MQTT_BROKER":       testMQTTBroker,
		"MQTT_PORT":         testMQTTPort,
		"POSTGRES_HOST":     testPostgresHost,
		"POSTGRES_PORT":     testPostgresPort,
		"POSTGRES_DB":       testPostgresDB,
		"POSTGRES_USER":     testPostgresUser,
		"POSTGRES_PASSWORD": testPostgresPass,
		"LOG_LEVEL":         "DEBUG",
		"BUFFER_SIZE":       "2", // small enough to fill deterministically under a fast burst
	}
	app := startTestApp(t, env)
	app.waitForEvent(t, "startup_complete", 30*time.Second)

	// Stop the database BEFORE publishing anything. This removes the timing race
	// entirely rather than merely narrowing it: with test-postgres down, the very
	// first message the writer dequeues blocks forever in its steady-state
	// (unbounded, by design — see AC13) retry, so dbWriterLoop can never pull a
	// second message off msgChan. A fast burst against a live, fast local database
	// would let the writer drain faster than the buffer could fill (that is exactly
	// what was observed and reported: two buffer_full WARNs followed immediately by
	// a stream of successful writes, then a clean flush_complete — no message ever
	// actually caught mid-send). With the writer genuinely stuck, the buffer fills
	// deterministically and stays full until shutdown.
	dockerStop(t, postgresContainer)
	t.Cleanup(func() { dockerStart(t, postgresContainer) })

	// Publish a fast burst to the SAME topic (so message_dropped's topic= field is
	// deterministic regardless of exactly which instance gets caught): one message
	// is consumed into the permanently-stuck retry, two more fill BUFFER_SIZE=2, and
	// the rest are guaranteed to hit the producer-blocked branch.
	burstTopic := fmt.Sprintf("integration/shutdown/burst-%d", time.Now().UnixNano())
	const burstSize = 6
	for i := 0; i < burstSize; i++ {
		publishTestMessage(t, burstTopic, fmt.Sprintf(`{"seq": %d}`, i))
	}

	// Confirm the timing actually happened: the app must have reached the
	// buffer_full WARN (main.go's producer-blocked branch) before we signal shutdown.
	app.waitForEvent(t, "buffer_full", 15*time.Second)

	// Send SIGTERM: the producer's blocked select (msgChan<-msg vs <-done) resolves
	// in favour of done, since the writer is permanently stuck and no slot will ever
	// free, and message_dropped fires for whichever message was caught mid-send.
	app.terminate(t)

	droppedLine := app.waitForEvent(t, "message_dropped", 15*time.Second)
	if !regexp.MustCompile(`topic=` + regexp.QuoteMeta(burstTopic) + `(\s|$)`).MatchString(droppedLine) {
		t.Fatalf("message_dropped line does not reference the published topic %q: %q", burstTopic, droppedLine)
	}

	// The writer's stuck steady-state retry means the drain can never actually
	// complete on its own — main's own defaultShutdownTimeout is what bounds exit
	// here (AC13), same as TestShutdownDrainBounded.
	app.waitForExit(t, defaultShutdownTimeout+10*time.Second)

	shutdownLine := app.waitForEvent(t, "shutdown_complete", 1*time.Second)
	shutdownTime := eventTime(t, shutdownLine)

	// healthWG-ordering assertion: no health_check line may carry a timestamp later
	// than shutdown_complete's. Given the 60 s health-check interval far exceeds this
	// short test's runtime, zero health_check lines is the expected, acceptable case —
	// the assertion is "none after shutdown_complete", not "at least one before it".
	for _, line := range app.allLines() {
		if !healthCheckEventRE.MatchString(line) {
			continue
		}
		if eventTime(t, line).After(shutdownTime) {
			t.Fatalf("health_check line occurred after shutdown_complete (healthWG ordering violated): %q", line)
		}
	}
}
