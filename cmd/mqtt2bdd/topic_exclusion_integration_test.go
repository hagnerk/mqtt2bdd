//go:build integration

package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"testing"
	"time"
)

// TestTopicExclusion proves that a message whose topic matches MQTT_EXCLUDE_TOPICS is
// dropped before the buffer and logged at DEBUG, while other topics are still stored. The
// sibling topic shares the excluded root as a string prefix but not as a level, so a
// prefix-based matcher would wrongly exclude it.
func TestTopicExclusion(t *testing.T) {
	suffix := time.Now().UnixNano()
	excludedRoot := fmt.Sprintf("integration/exclusion/excluded-%d", suffix)
	filter := excludedRoot + "/#"
	unusedFilter := fmt.Sprintf("integration/exclusion/unused-%d/+", suffix)

	env := map[string]string{
		"MQTT_BROKER":       testMQTTBroker,
		"MQTT_PORT":         testMQTTPort,
		"POSTGRES_HOST":     testPostgresHost,
		"POSTGRES_PORT":     testPostgresPort,
		"POSTGRES_DB":       testPostgresDB,
		"POSTGRES_USER":     testPostgresUser,
		"POSTGRES_PASSWORD": testPostgresPass,
		// message_excluded is DEBUG. Blanks and an empty entry exercise the parsing end-to-end.
		"LOG_LEVEL":           "DEBUG",
		"MQTT_EXCLUDE_TOPICS": " " + filter + " ,, " + unusedFilter,
	}
	app := startTestApp(t, env)
	app.waitForEvent(t, "startup_complete", 30*time.Second)

	configLoaded := app.waitForEvent(t, "config_loaded", logCaptureTimeout)
	wantList := regexp.MustCompile(`\sexclude_topics=` + regexp.QuoteMeta(filter+","+unusedFilter) + `(\s|$)`)
	if !wantList.MatchString(configLoaded) {
		t.Errorf("config_loaded must report exclude_topics=%s,%s: %q", filter, unusedFilter, configLoaded)
	}

	pool := openTestDBPool(t)

	excludedChild := excludedRoot + "/bridge/state"
	excludedParent := excludedRoot
	storedSibling := excludedRoot + "-sibling"
	storedOther := fmt.Sprintf("integration/exclusion/stored-%d", suffix)

	// QoS 0 and never retained: a retained message would be redelivered to every later test.
	for _, topic := range []string{excludedChild, excludedParent, storedSibling, storedOther} {
		publishTestMessage(t, topic, `{"value": 1}`)
	}

	for _, topic := range []string{storedSibling, storedOther} {
		if !waitForRow(pool, topic, defaultRetryIntervalOnDatabaseFailure/2) {
			t.Fatalf("message on %s was not persisted within %s", topic, defaultRetryIntervalOnDatabaseFailure/2)
		}
	}

	excluded := waitForEventCount(t, app, "message_excluded", 2)
	filterRe := regexp.MustCompile(`\sfilter=` + regexp.QuoteMeta(filter) + `(\s|$)`)
	for _, line := range excluded {
		if !regexp.MustCompile(`(^|\s)level=DEBUG\s`).MatchString(line) {
			t.Errorf("message_excluded entry must be DEBUG: %q", line)
		}
		if !filterRe.MatchString(line) {
			t.Errorf("message_excluded entry must carry filter=%s: %q", filter, line)
		}
		if regexp.MustCompile(`\soperation=`).MatchString(line) {
			t.Errorf("message_excluded reports a state and must carry no operation: %q", line)
		}
		if regexp.MustCompile(`\spayload=`).MatchString(line) {
			t.Errorf("message_excluded entry must never carry the payload body: %q", line)
		}
	}
	for _, topic := range []string{excludedChild, excludedParent} {
		if n := countTopicLines(excluded, topic); n != 1 {
			t.Errorf("expected exactly one message_excluded entry for %s, got %d:\n%v", topic, n, excluded)
		}
	}
	for _, topic := range []string{storedSibling, storedOther} {
		if n := countTopicLines(excluded, topic); n != 0 {
			t.Errorf("a stored topic must not be logged as excluded: %s\n%v", topic, excluded)
		}
	}

	// Checked only now: both message_excluded lines prove the handler has already run for
	// the excluded messages, so an absent row cannot be a row that is merely late.
	var count int
	err := pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM sensor_metrics WHERE sensor = ANY($1)",
		[]string{excludedChild, excludedParent}).Scan(&count)
	if err != nil {
		t.Fatalf("verification query failed: %v", err)
	}
	if count != 0 {
		t.Errorf("excluded topics must not be stored, found %d rows", count)
	}

	app.terminate(t)
	app.waitForExit(t, defaultShutdownTimeout+5*time.Second)
}

// TestTopicExclusion_InvalidFilter proves that an invalid filter stops the application at
// startup with config_load_failed naming the filter, before any connection is made.
func TestTopicExclusion_InvalidFilter(t *testing.T) {
	const invalidFilter = "sport/tennis/#/ranking"
	env := map[string]string{
		"MQTT_BROKER":         testMQTTBroker,
		"MQTT_PORT":           testMQTTPort,
		"POSTGRES_HOST":       testPostgresHost,
		"POSTGRES_PORT":       testPostgresPort,
		"POSTGRES_DB":         testPostgresDB,
		"POSTGRES_USER":       testPostgresUser,
		"POSTGRES_PASSWORD":   testPostgresPass,
		"MQTT_EXCLUDE_TOPICS": "valid/#," + invalidFilter,
	}
	app := startTestApp(t, env)

	select {
	case <-app.exited:
	case <-time.After(10 * time.Second):
		t.Fatalf("application did not exit within 10s on an invalid filter")
	}
	var exitErr *exec.ExitError
	if !errors.As(app.waitErr, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("application must exit with code 1, got: %v", app.waitErr)
	}

	// Polled: the stdout drain goroutine can still be appending when exited closes.
	app.waitForEvent(t, "config_load_failed", logCaptureTimeout)
	failed := app.eventLines("config_load_failed")
	if len(failed) != 1 {
		t.Fatalf("expected exactly one config_load_failed entry, got %d:\n%v", len(failed), app.allLines())
	}
	line := failed[0]
	if !regexp.MustCompile(`(^|\s)level=ERROR\s`).MatchString(line) {
		t.Errorf("config_load_failed entry must be ERROR: %q", line)
	}
	// slog escapes the quotes %q adds, so match the filter itself.
	if !regexp.MustCompile(regexp.QuoteMeta(invalidFilter)).MatchString(line) {
		t.Errorf("config_load_failed entry must name %s: %q", invalidFilter, line)
	}
	if app.hasEvent("starting") {
		t.Errorf("the application must abort before logging starting:\n%v", app.allLines())
	}
}

// countTopicLines returns how many lines carry exactly topic=topic.
func countTopicLines(lines []string, topic string) int {
	re := regexp.MustCompile(`\stopic=` + regexp.QuoteMeta(topic) + `(\s|$)`)
	n := 0
	for _, line := range lines {
		if re.MatchString(line) {
			n++
		}
	}
	return n
}
