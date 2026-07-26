#!/bin/sh
# Integration test runner for mqtt2bdd (AC7). Brings up the isolated test stack, runs a
# basic pub->persist smoke test against the compose-managed app, then runs the full Go
# integration suite (which exercises FIND-010/FIND-016/message_dropped scenarios via its
# own subprocesses), then tears everything down unconditionally (AC10). Exit code is 0 on
# success, non-zero on any failure (AC9), for CI/CD use.
#
# Run from the repository root: ./test/run-integration-tests.sh

set -u

COMPOSE="docker compose -f test/docker-compose.yml"

cleanup() {
  echo "==> Tearing down test stack"
  $COMPOSE down -v --remove-orphans
}
trap cleanup EXIT INT TERM

echo "==> Building and starting test stack"
if ! $COMPOSE up -d --build; then
  echo "FAIL: test stack did not start"
  exit 1
fi

echo "==> Waiting for all services to report healthy"
deadline=$(( $(date +%s) + 90 ))
while true; do
  unhealthy=$($COMPOSE ps --format '{{.Name}} {{.Health}}' | grep -v ' healthy$' || true)
  if [ -z "$unhealthy" ]; then
    break
  fi
  if [ "$(date +%s)" -ge "$deadline" ]; then
    echo "FAIL: services did not become healthy within 90s:"
    echo "$unhealthy"
    exit 1
  fi
  sleep 2
done
echo "All services healthy."

echo "==> Basic smoke test: publish -> persist -> verify"
if ! $COMPOSE exec -T mosquitto mosquitto_pub -h localhost -t 'integration/smoke' -m '{"value": 42}'; then
  echo "FAIL: mosquitto_pub failed"
  exit 1
fi
sleep 2
rows=$($COMPOSE exec -T postgres psql -U mqtt2bdd -d mqtt2bdd -t -c \
  "SELECT COUNT(*) FROM sensor_metrics WHERE sensor='integration/smoke';" | tr -d '[:space:]')
if [ "$rows" -lt 1 ] 2>/dev/null; then
  echo "FAIL: smoke-test message not found in sensor_metrics (rows=$rows)"
  exit 1
fi
echo "Smoke test passed: message persisted."

echo "==> Stopping compose-managed mqtt2bdd (releases the MQTT client ID for the Go test suite)"
$COMPOSE stop mqtt2bdd

echo "==> Running Go integration test suite"
# The docker socket is mounted into this throwaway container, and the Docker CLI is
# installed before the suite runs, because AC12/AC13's tests must themselves invoke
# `docker stop`/`docker start` against mqtt2bdd-test-mosquitto/mqtt2bdd-test-postgres
# (a real broker/database restart, not a mock) from inside the process that runs them.
# Talking to the host daemon's socket means these commands operate on the containers by
# name regardless of which network namespace this container itself is attached to.
#
# `go test -tags=integration ./...` builds every integration-tagged package in the
# module, not just cmd/mqtt2bdd's — including the pre-existing internal/database
# integration suite (Story 1.8), whose TestMain calls config.LoadConfig() and requires
# these environment variables directly (os.Getenv), independent of cmd/mqtt2bdd's own
# subprocess env. Values match test/docker-compose.yml's mqtt2bdd service.
if ! docker run --rm \
    --network mqtt2bdd-test-network \
    -v "$(pwd):/app" -w /app \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -e MQTT_BROKER=mqtt2bdd-test-mosquitto \
    -e MQTT_PORT=1883 \
    -e POSTGRES_HOST=mqtt2bdd-test-postgres \
    -e POSTGRES_PORT=5432 \
    -e POSTGRES_DB=mqtt2bdd \
    -e POSTGRES_USER=mqtt2bdd \
    -e POSTGRES_PASSWORD=test_password \
    -e LOG_LEVEL=DEBUG \
    golang:1.23-alpine \
    sh -c 'apk add --no-cache docker-cli >/dev/null && go test -tags=integration -p 1 -v ./...'; then
  echo "FAIL: Go integration test suite failed"
  exit 1
fi

echo "==> All integration tests passed."
