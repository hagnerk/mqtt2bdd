//go:build integration

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Connection details for the isolated test stack (test/docker-compose.yml). This test
// binary runs inside the throwaway golang:1.23-alpine container run-integration-tests.sh
// attaches to mqtt2bdd-test-network, so these hostnames resolve via container DNS — no
// host.docker.internal, no dependency on the published host ports.
const (
	testMQTTBroker     = "mqtt2bdd-test-mosquitto"
	testMQTTPort       = "1883"
	testPostgresHost   = "mqtt2bdd-test-postgres"
	testPostgresPort   = "5432"
	testPostgresDB     = "mqtt2bdd"
	testPostgresUser   = "mqtt2bdd"
	testPostgresPass   = "test_password"
	mosquittoContainer = "mqtt2bdd-test-mosquitto"
	postgresContainer  = "mqtt2bdd-test-postgres"
)

// testDSN returns the pgx connection string for the test-postgres container.
func testDSN() string {
	return fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s",
		testPostgresHost, testPostgresPort, testPostgresDB, testPostgresUser, testPostgresPass)
}

// TestMain truncates sensor_metrics once before this package's integration tests run,
// so a leftover row from a previous run cannot masquerade as evidence for waitForRow.
// The cmd/mqtt2bdd integration tests are deliberately NOT run with t.Parallel() (see the
// story's "MQTT ClientID Constraint": every subprocess spawned here reuses the same
// hardcoded Paho client ID, so two live instances against the same broker collide) —
// Go runs this package's top-level tests sequentially in source order by default.
func TestMain(m *testing.M) {
	pool, err := pgxpool.New(context.Background(), testDSN())
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: open pool: %v\n", err)
		os.Exit(1)
	}
	if _, err := pool.Exec(context.Background(), "TRUNCATE TABLE sensor_metrics"); err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: truncate before tests: %v\n", err)
		os.Exit(1)
	}
	pool.Close()

	os.Exit(m.Run())
}

var (
	buildBinaryOnce sync.Once
	testBinaryPath  string
	testBinaryErr   error
)

// buildTestBinary compiles cmd/mqtt2bdd exactly once for the whole package, caching the
// resulting path. Building via the full import path (rather than a relative "./...")
// means it works regardless of which package directory "go test" happens to set as the
// process's working directory.
func buildTestBinary(t *testing.T) string {
	t.Helper()
	buildBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "mqtt2bdd-integration-*")
		if err != nil {
			testBinaryErr = fmt.Errorf("create temp dir: %w", err)
			return
		}
		testBinaryPath = dir + "/mqtt2bdd"
		cmd := exec.Command("go", "build", "-o", testBinaryPath, "github.com/hagnerk/mqtt2bdd/cmd/mqtt2bdd")
		out, err := cmd.CombinedOutput()
		if err != nil {
			testBinaryErr = fmt.Errorf("go build: %w\n%s", err, out)
		}
	})
	if testBinaryErr != nil {
		t.Fatalf("buildTestBinary: %v", testBinaryErr)
	}
	return testBinaryPath
}

// testApp wraps a running mqtt2bdd subprocess, capturing its stdout line-by-line so
// assertions can wait for a specific structured-log event to appear.
type testApp struct {
	cmd     *exec.Cmd
	mu      sync.Mutex
	lines   []string
	exited  chan struct{}
	waitErr error
}

// startTestApp builds (once per package) and starts mqtt2bdd as a subprocess configured
// by env, draining its stdout into an in-memory line buffer as it arrives so the pipe
// never blocks the child.
func startTestApp(t *testing.T, env map[string]string) *testApp {
	t.Helper()
	bin := buildTestBinary(t)

	cmd := exec.Command(bin)
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start subprocess: %v", err)
	}

	app := &testApp{cmd: cmd, exited: make(chan struct{})}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			app.mu.Lock()
			app.lines = append(app.lines, line)
			app.mu.Unlock()
			fmt.Fprintf(os.Stderr, "[app] %s\n", line)
		}
	}()

	go func() {
		app.waitErr = cmd.Wait()
		close(app.exited)
	}()

	t.Cleanup(func() {
		select {
		case <-app.exited:
		default:
			_ = cmd.Process.Kill()
			<-app.exited
		}
	})

	return app
}

// allLines returns a snapshot copy of every stdout line captured so far.
func (a *testApp) allLines() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, len(a.lines))
	copy(out, a.lines)
	return out
}

// waitForEvent polls the captured stdout until a line with the given event= value
// appears, or fails the test after timeout. Returns the matching line.
func (a *testApp) waitForEvent(t *testing.T, event string, timeout time.Duration) string {
	t.Helper()
	re := regexp.MustCompile(`event=` + regexp.QuoteMeta(event) + `(\s|$)`)
	deadline := time.Now().Add(timeout)
	for {
		for _, line := range a.allLines() {
			if re.MatchString(line) {
				return line
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for event=%s in subprocess stdout", timeout, event)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// hasEvent reports whether any captured line so far carries event=event, without
// waiting or failing the test.
func (a *testApp) hasEvent(event string) bool {
	re := regexp.MustCompile(`event=` + regexp.QuoteMeta(event) + `(\s|$)`)
	for _, line := range a.allLines() {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

// terminate sends SIGTERM to the subprocess, mirroring the SIGTERM a real deployment's
// container stop delivers.
func (a *testApp) terminate(t *testing.T) {
	t.Helper()
	if err := a.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}
}

// waitForExit blocks until the subprocess exits or timeout elapses, and fails the test
// if the process exited with a non-zero status.
func (a *testApp) waitForExit(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-a.exited:
	case <-time.After(timeout):
		t.Fatalf("subprocess did not exit within %s", timeout)
		return
	}
	if a.waitErr != nil {
		t.Fatalf("subprocess exited with error (want exit 0): %v", a.waitErr)
	}
}

// eventTime extracts and parses the time= field of a structured log line emitted by
// internal/logger (slog.NewTextHandler, RFC3339 with a "Z" suffix, UTC).
func eventTime(t *testing.T, line string) time.Time {
	t.Helper()
	re := regexp.MustCompile(`time=(\S+)`)
	m := re.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("no time= field found in line: %q", line)
	}
	tm, err := time.Parse(time.RFC3339Nano, m[1])
	if err != nil {
		t.Fatalf("failed to parse time=%q in line %q: %v", m[1], line, err)
	}
	return tm
}

// publisherClientID guarantees every test publisher connection uses a distinct Paho
// client ID, so a rapid sequence of publishes across subtests never collides even
// though the application under test always uses the hardcoded "mqtt2bdd" ID.
var publisherSeq int64

func nextPublisherClientID() string {
	publisherSeq++
	return fmt.Sprintf("integration-test-publisher-%d-%d", time.Now().UnixNano(), publisherSeq)
}

// publishTestMessage publishes payload to topic on the test broker using a short-lived
// Paho client distinct from the application under test.
func publishTestMessage(t *testing.T, topic, payload string) {
	t.Helper()
	opts := pahomqtt.NewClientOptions().
		AddBroker(fmt.Sprintf("tcp://%s:%s", testMQTTBroker, testMQTTPort)).
		SetClientID(nextPublisherClientID()).
		SetConnectRetry(false)
	client := pahomqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(10 * time.Second) {
		t.Fatalf("publisher connect to %s timed out", testMQTTBroker)
	}
	if err := token.Error(); err != nil {
		t.Fatalf("publisher connect: %v", err)
	}
	defer client.Disconnect(250)

	pubToken := client.Publish(topic, 0, false, payload)
	if !pubToken.WaitTimeout(10 * time.Second) {
		t.Fatalf("publish to %s timed out", topic)
	}
	if err := pubToken.Error(); err != nil {
		t.Fatalf("publish to %s: %v", topic, err)
	}
}

// openTestDBPool opens a connection pool to the test-postgres container, independent
// of the application under test, for verifying rows it wrote.
func openTestDBPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testDSN())
	if err != nil {
		t.Fatalf("open test db pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// waitForRow polls sensor_metrics until at least one row for sensor exists, or timeout
// elapses. Returns whether the row was found.
func waitForRow(pool *pgxpool.Pool, sensor string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		var count int
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM sensor_metrics WHERE sensor = $1", sensor).Scan(&count)
		cancel()
		if err == nil && count >= 1 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// dockerStop stops a running container by name via the host Docker daemon, reached
// through the docker socket mounted into this test-runner container (see
// test/run-integration-tests.sh). A restart, not a network partition: this is the
// only way to reproduce Mosquitto genuinely forgetting subscriptions server-side
// (AC12/FIND-010), and the only way to make test-postgres briefly unavailable to a
// running writer mid-drain (AC13/FIND-016).
func dockerStop(t *testing.T, container string) {
	t.Helper()
	out, err := exec.Command("docker", "stop", container).CombinedOutput()
	if err != nil {
		t.Fatalf("docker stop %s: %v\n%s", container, err, out)
	}
}

// dockerStart starts a previously-stopped container by name and blocks until Docker
// reports it healthy again. Waiting here (rather than returning as soon as the process
// starts) matters because "docker start" returning is not the same as the service inside
// being ready to accept connections — Postgres in particular takes a moment to open its
// listener, and main() makes exactly one connection attempt at startup with no retry, so
// a subprocess started immediately after an unhealthy restart fails startup outright.
func dockerStart(t *testing.T, container string) {
	t.Helper()
	out, err := exec.Command("docker", "start", container).CombinedOutput()
	if err != nil {
		t.Fatalf("docker start %s: %v\n%s", container, err, out)
	}
	waitForContainerHealthy(t, container, 30*time.Second)
}

// waitForContainerHealthy polls the container's Docker healthcheck status until it
// reports "healthy", or fails the test after timeout.
func waitForContainerHealthy(t *testing.T, container string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastStatus string
	var lastErr error
	for {
		out, err := exec.Command("docker", "inspect", "--format", "{{.State.Health.Status}}", container).Output()
		lastStatus = strings.TrimSpace(string(out))
		lastErr = err
		if err == nil && lastStatus == "healthy" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("container %s did not become healthy within %s (last status=%q, err=%v)", container, timeout, lastStatus, lastErr)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
