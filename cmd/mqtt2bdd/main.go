// Package main is the MQTT2BDD application entry point. It reads configuration
// from the environment, connects to the MQTT broker and PostgreSQL, then
// decouples message reception from database writes through a buffered channel
// (the Go CSP pattern): a database-writer goroutine consumes and persists
// messages while a separate health-check goroutine periodically logs
// connection and buffer status. On SIGTERM/SIGINT it stops accepting new
// messages, drains whatever is still buffered (bounded by a shutdown timeout),
// and only then exits.
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spydemon/mqtt2bdd/internal/config"
	"github.com/spydemon/mqtt2bdd/internal/database"
	"github.com/spydemon/mqtt2bdd/internal/logger"
	"github.com/spydemon/mqtt2bdd/internal/mqtt"
)

// defaultRetryIntervalOnDatabaseFailure is the delay between insert retries in the DB writer goroutine.
const defaultRetryIntervalOnDatabaseFailure = 10 * time.Second

// defaultShutdownTimeout bounds how long the shutdown sequence waits for buffered
// messages to drain before giving up. Aligns with the Docker stop grace period.
const defaultShutdownTimeout = 30 * time.Second

// defaultInsertAttemptTimeout bounds ONE insert attempt so a hung (not refused)
// database cannot block a retry loop forever, in either state.
const defaultInsertAttemptTimeout = 5 * time.Second

// defaultConnectTimeout bounds startup connection attempts so a hung broker or
// database cannot block startup indefinitely.
const defaultConnectTimeout = 10 * time.Second

// defaultHealthCheckInterval is the cadence of the health status log. One minute is
// frequent enough to notice an outage promptly, and rare enough that a year of uptime
// is a manageable number of entries.
const defaultHealthCheckInterval = 60 * time.Second

// bufferHighWaterMarkPercent is the buffer utilization above which backpressure is
// reported: past this point the database is not keeping up with the broker, and the
// remaining headroom is the operator's window to react before messages block.
const bufferHighWaterMarkPercent = 80.0

// Version is injected at build time via ldflags. Defaults to "dev" for local builds.
var Version = "dev"

// insertWithRetry writes a single message, retrying every defaultRetryIntervalOnDatabaseFailure
// until the insert succeeds, so no message is lost during a database outage. ctx bounds each
// individual insert attempt (defaultInsertAttemptTimeout) and, when it carries a deadline (the
// shutdown drain path), also bounds how long the retry loop as a whole may run: steady-state
// callers pass context.Background(), which never expires, so normal-operation retries stay
// unbounded — only the drain path's ctx.Done() case is ever reachable.
func insertWithRetry(ctx context.Context, dbClient *database.Client, msg mqtt.Message) {
	for {
		attemptCtx, cancel := context.WithTimeout(ctx, defaultInsertAttemptTimeout)
		err := dbClient.InsertMessage(attemptCtx, msg.Topic, msg.Timestamp, json.RawMessage(msg.Payload))
		cancel()
		if err == nil {
			return
		}
		// error already logged by InsertMessage; hold this message and retry after delay,
		// unless ctx itself has run out of budget (only possible on the drain path).
		select {
		case <-time.After(defaultRetryIntervalOnDatabaseFailure):
		case <-ctx.Done():
			return
		}
	}
}

// dbWriterLoop is the sole consumer of msgChan. It writes each message (retrying until success,
// ensuring no message is lost during a database outage). On shutdown, done is closed: the loop
// drains any messages still buffered and then exits. msgChan is never closed, so producers can
// never panic with "send on closed channel". shutdownCtx bounds only the drain branch — the
// steady-state branch always passes context.Background(), so the "no message lost during a
// database outage" invariant from Stories 1.8/2.3 is unchanged in normal operation.
func dbWriterLoop(msgChan <-chan mqtt.Message, dbClient *database.Client, done <-chan struct{}, shutdownCtx context.Context) {
	for {
		select {
		case msg := <-msgChan:
			insertWithRetry(context.Background(), dbClient, msg)
		case <-done:
			// Shutdown requested: drain the remaining buffered messages, then exit.
			for {
				select {
				case msg := <-msgChan:
					insertWithRetry(shutdownCtx, dbClient, msg)
				default:
					return
				}
			}
		}
	}
}

// connectionStatus renders a connection state as the vocabulary the health entry uses.
// The two literals exist here and nowhere else, so MQTT and database read identically.
func connectionStatus(connected bool) string {
	if connected {
		return "connected"
	}
	return "disconnected"
}

// bufferUtilizationPercent returns used/capacity as a percentage rounded to one decimal.
// The zero guard is load-bearing: BUFFER_SIZE=0 is accepted and yields an unbuffered
// channel, where the division would be 0/0 and produce NaN. Rounding keeps the field a
// number a log processor can aggregate rather than 33.333333333333336.
func bufferUtilizationPercent(used, capacity int) float64 {
	if capacity == 0 {
		return 0
	}
	p := float64(used) / float64(capacity) * 100
	return math.Round(p*10) / 10
}

// writeAgeSeconds returns the number of whole seconds between lastWrite and now, or -1
// if lastWrite is the zero time.Time (no successful write has happened yet). -1 rather
// than an omitted field: an absent value would be indistinguishable from a parsing gap,
// where the sentinel is greppable.
func writeAgeSeconds(lastWrite, now time.Time) int64 {
	if lastWrite.IsZero() {
		return -1
	}
	return int64(now.Sub(lastWrite).Seconds())
}

// healthCheckLoop logs one health status entry per tick until done is closed, plus a
// separate WARN whenever the buffer sits above its high-water mark. It only reads
// msgChan's length and capacity: it never sends, receives or closes.
func healthCheckLoop(l *slog.Logger, mqttClient *mqtt.Client, dbClient *database.Client, msgChan <-chan mqtt.Message, done <-chan struct{}) {
	// A ticker rather than a sleep loop, so the interval does not drift by the cost
	// of each iteration.
	ticker := time.NewTicker(defaultHealthCheckInterval)
	// defer for cleanup: Go's standard idiom for guaranteed release of a resource
	// on function return, regardless of which exit path is taken (both select
	// branches below return from this function). Stopping the ticker here means
	// there is exactly one place that must be kept in sync with the function's
	// exit points, rather than one release call per return statement.
	defer ticker.Stop()

	var lastWriteCount uint64

	for {
		select {
		case <-ticker.C:
			used := len(msgChan)
			capacity := cap(msgChan)
			utilization := bufferUtilizationPercent(used, capacity)

			// WriteCount is monotonic, so this unsigned subtraction cannot wrap.
			total := dbClient.WriteCount()
			processed := total - lastWriteCount
			lastWriteCount = total

			lastWriteAgeSeconds := writeAgeSeconds(dbClient.LastWriteAt(), time.Now())

			l.Info("health check",
				"event", "health_check",
				"mqtt_status", connectionStatus(mqttClient.IsConnected()),
				"db_status", connectionStatus(dbClient.IsConnected()),
				"db_last_write_age_s", lastWriteAgeSeconds,
				"buffer_used", used,
				"buffer_capacity", capacity,
				"buffer_utilization_percent", utilization,
				"processed_last_interval", processed,
			)

			// A separate entry, not a field on the line above: severity is a property
			// of an entry, and an operator filtering on level=WARN must see this one.
			// Strictly greater than the mark — a buffer at exactly 80% is not yet degraded.
			if utilization > bufferHighWaterMarkPercent {
				l.Warn("message buffer above high-water mark, possible backpressure",
					"event", "buffer_high",
					"buffer_used", used,
					"buffer_capacity", capacity,
					"buffer_utilization_percent", utilization,
				)
			}
		case <-done:
			return
		}
	}
}

func main() {
	// Created before the configuration is read so the config-failure path is
	// structured too; an empty level means INFO, silently.
	bootstrapLogger := logger.WithComponent(logger.InitLogger(""), logger.ComponentMain)

	cfg, err := config.LoadConfig()
	if err != nil {
		bootstrapLogger.Error("configuration error", "event", "config_load_failed", "operation", "load_config", "error", err)
		os.Exit(1)
	}

	// baseLogger stays untagged: each client attaches its own component, so handing it
	// an already-tagged logger would emit "component" twice on every line it writes.
	baseLogger := logger.InitLogger(cfg.LogLevel)
	l := logger.WithComponent(baseLogger, logger.ComponentMain)
	l.Info("MQTT2BDD starting", "event", "starting", "version", Version)
	// Both levels are reported: log_level is what the operator asked for, effective_log_level
	// is what the handler actually filters on, so a typo is diagnosable from this one entry.
	l.Info("configuration loaded", "event", "config_loaded", "log_level", cfg.LogLevel, "effective_log_level", logger.EffectiveLevel(cfg.LogLevel).String(), "buffer_size", cfg.BufferSize)

	mqttClient := mqtt.NewClient(cfg, baseLogger)
	mqttCtx, mqttCancel := context.WithTimeout(context.Background(), defaultConnectTimeout)
	err = mqttClient.Connect(mqttCtx)
	mqttCancel()
	if err != nil {
		// failed_component, not component: this entry is emitted by main, and component
		// must keep identifying the emitter rather than the culprit.
		l.Error("startup aborted", "event", "startup_aborted", "operation", "connect", "failed_component", logger.ComponentMQTT, "error", err)
		os.Exit(1)
	}

	dbClient := database.NewClient(cfg, baseLogger)
	dbCtx, dbCancel := context.WithTimeout(context.Background(), defaultConnectTimeout)
	err = dbClient.Connect(dbCtx)
	dbCancel()
	if err != nil {
		l.Error("startup aborted", "event", "startup_aborted", "operation", "connect", "failed_component", logger.ComponentDatabase, "error", err)
		os.Exit(1)
	}

	// msgChan decouples MQTT message reception from database writes (Go CSP pattern).
	// Producer: MQTT message handler (Paho callback goroutine).
	// Consumer: DB writer goroutine below.
	msgChan := make(chan mqtt.Message, cfg.BufferSize)
	l.Info("message buffer initialised", "event", "buffer_initialised", "buffer_capacity", cfg.BufferSize)

	// done is closed once at shutdown to signal both the DB writer (drain and exit) and any
	// producer blocked on a full buffer (stop waiting). msgChan itself is never closed.
	done := make(chan struct{})

	// shutdownCtx bounds only the drain path (dbWriterLoop's done branch). Created with
	// WithCancel rather than WithTimeout: WithCancel costs nothing until cancelled, so
	// creating it here, long before any shutdown signal, is free — a WithTimeout's clock
	// would start immediately and expire long before a real SIGTERM arrives.
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()

	// DB writer goroutine: sole consumer of msgChan, drains and exits when done is closed.
	// The WaitGroup lets the shutdown sequence wait for the loop to drain and finish.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		dbWriterLoop(msgChan, dbClient, done, shutdownCtx)
	}()
	l.Info("database writer started", "event", "writer_started")

	// Health monitor goroutine. It gets its own WaitGroup rather than joining wg,
	// because wg is what the drained channel signals: adding the health loop would
	// turn "the DB writer finished flushing" into "…and the health loop stopped too",
	// which is exactly the conflation flush_complete / flush_timeout exists to avoid.
	var healthWG sync.WaitGroup
	healthWG.Add(1)
	go func() {
		defer healthWG.Done()
		healthCheckLoop(l, mqttClient, dbClient, msgChan, done)
	}()
	l.Info("health monitor started", "event", "health_monitor_started", "interval_s", int(defaultHealthCheckInterval.Seconds()))

	mqttMessageHandler := func(topic string, payload []byte) {
		msg := mqtt.Message{Topic: topic, Timestamp: time.Now(), Payload: payload}
		select {
		case msgChan <- msg:
		default:
			l.Warn("message buffer full, blocking until space available", "event", "buffer_full", "buffer_size", cfg.BufferSize)
			// Block until the DB writer frees a slot, or shutdown begins (drop the message
			// rather than block on a channel that is draining for the last time).
			select {
			case msgChan <- msg:
			case <-done:
				l.Warn("message dropped at shutdown, buffer was full", "event", "message_dropped", "topic", topic)
			}
		}
	}
	if err := mqttClient.Subscribe("#", mqttMessageHandler); err != nil {
		l.Error("MQTT subscribe failed", "event", "startup_aborted", "operation", "subscribe", "failed_component", logger.ComponentMQTT, "error", err)
		os.Exit(1)
	}

	l.Info("startup complete", "event", "startup_complete", "version", Version)

	// Block until a shutdown signal arrives. Buffered (cap 1) so the signal is not
	// missed if it fires before this goroutine reaches the receive.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	sig := <-sigChan
	l.Info("Shutdown signal received, flushing buffered messages", "event", "shutdown_signal", "signal", sig.String())

	// Ordered teardown (order is required for correctness, not stylistic):
	mqttClient.Disconnect()  // Stop receiving new messages before signalling the writer.
	buffered := len(msgChan) // No producers remain, so this is the count awaiting flush.
	close(done)              // Signal the DB writer to drain and exit, and unblock any producer.
	// Arms the SAME 30s budget the select below already races, from the same starting
	// instant — the two converge instead of racing independently.
	time.AfterFunc(defaultShutdownTimeout, shutdownCancel)

	// Wait for the writer to finish, racing the shutdown timeout.
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		wg.Wait()
	}()
	select {
	case <-drained:
		l.Info("Flushed buffered messages", "event", "flush_complete", "count", buffered)
	case <-time.After(defaultShutdownTimeout):
		l.Warn("Shutdown timeout exceeded; some buffered messages may not have been flushed", "event", "flush_timeout", "buffered", buffered)
	}

	// The health loop is parked in a select and returns within microseconds of close(done),
	// so this never delays shutdown; waiting here keeps teardown deterministic, with no
	// goroutine still reading the client when its pool is torn down.
	healthWG.Wait()

	dbClient.Close()                                          // Close the pool only after the drain resolves.
	l.Info("Shutdown complete", "event", "shutdown_complete") // Natural return ⇒ exit code 0.
}
