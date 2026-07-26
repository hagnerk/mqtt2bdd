//go:build integration

package main

import (
	"fmt"
	"testing"
	"time"
)

// TestMQTTReconnect reproduces the observable, user-visible half of FIND-010 (AC12)
// against the real test stack: after a genuine Mosquitto restart (docker stop/start,
// which forgets every subscription server-side — not a network partition), a message
// published post-restart must still be received and persisted. Before the reconnectLoop
// fix in internal/mqtt/client.go, this second message is silently lost forever, because
// the single re-subscribe attempt after reconnect fails once and is never retried.
func TestMQTTReconnect(t *testing.T) {
	pool := openTestDBPool(t)

	env := map[string]string{
		"MQTT_BROKER":       testMQTTBroker,
		"MQTT_PORT":         testMQTTPort,
		"POSTGRES_HOST":     testPostgresHost,
		"POSTGRES_PORT":     testPostgresPort,
		"POSTGRES_DB":       testPostgresDB,
		"POSTGRES_USER":     testPostgresUser,
		"POSTGRES_PASSWORD": testPostgresPass,
		"LOG_LEVEL":         "DEBUG",
		"BUFFER_SIZE":       "1000",
	}
	app := startTestApp(t, env)
	app.waitForEvent(t, "startup_complete", 30*time.Second)

	// 1. Publish and verify a message flows normally before any disruption.
	before := fmt.Sprintf("integration/reconnect/before-%d", time.Now().UnixNano())
	publishTestMessage(t, before, `{"value": 1}`)
	if !waitForRow(pool, before, 15*time.Second) {
		t.Fatalf("message published before broker restart was never persisted: sensor=%s", before)
	}

	// 2. Restart the broker for real. This is what makes Subscribe's retry load-bearing:
	// a plain network partition would let Paho's session state survive; a process
	// restart with mosquitto.conf's defaults does not.
	dockerStop(t, mosquittoContainer)
	dockerStart(t, mosquittoContainer)

	// 3. Wait for the app to notice, reconnect, and re-subscribe.
	app.waitForEvent(t, "reconnected", 30*time.Second)
	app.waitForEvent(t, "subscribed", 30*time.Second)

	// 4. Publish a second, distinctly-tagged message. Before the AC12 fix, this
	// message is silently dropped: the app is connected but not subscribed, and
	// health_check would report mqtt_status=connected the whole time regardless.
	after := fmt.Sprintf("integration/reconnect/after-%d", time.Now().UnixNano())
	publishTestMessage(t, after, `{"value": 2}`)
	if !waitForRow(pool, after, 20*time.Second) {
		t.Fatalf("message published after broker restart was never persisted (FIND-010 regression): sensor=%s", after)
	}

	app.terminate(t)
	app.waitForExit(t, 10*time.Second)
}
