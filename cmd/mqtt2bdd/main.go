package main

import (
	"context"
	"encoding/json"
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

// defaultConnectTimeout bounds startup connection attempts so a hung broker or
// database cannot block startup indefinitely.
const defaultConnectTimeout = 10 * time.Second

// Version is injected at build time via ldflags. Defaults to "dev" for local builds.
var Version = "dev"

// insertWithRetry writes a single message, retrying every defaultRetryIntervalOnDatabaseFailure
// until the insert succeeds, so no message is lost during a database outage.
func insertWithRetry(dbClient *database.Client, msg mqtt.Message) {
	for {
		err := dbClient.InsertMessage(context.Background(), msg.Topic, msg.Timestamp, json.RawMessage(msg.Payload))
		if err == nil {
			return
		}
		// error already logged by InsertMessage; hold this message and retry after delay
		time.Sleep(defaultRetryIntervalOnDatabaseFailure)
	}
}

// dbWriterLoop is the sole consumer of msgChan. It writes each message (retrying until success,
// ensuring no message is lost during a database outage). On shutdown, done is closed: the loop
// drains any messages still buffered and then exits. msgChan is never closed, so producers can
// never panic with "send on closed channel".
func dbWriterLoop(msgChan <-chan mqtt.Message, dbClient *database.Client, done <-chan struct{}) {
	for {
		select {
		case msg := <-msgChan:
			insertWithRetry(dbClient, msg)
		case <-done:
			// Shutdown requested: drain the remaining buffered messages, then exit.
			for {
				select {
				case msg := <-msgChan:
					insertWithRetry(dbClient, msg)
				default:
					return
				}
			}
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
	l.Info("configuration loaded", "event", "config_loaded", "log_level", cfg.LogLevel, "buffer_size", cfg.BufferSize)

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

	// DB writer goroutine: sole consumer of msgChan, drains and exits when done is closed.
	// The WaitGroup lets the shutdown sequence wait for the loop to drain and finish.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		dbWriterLoop(msgChan, dbClient, done)
	}()
	l.Info("database writer started", "event", "writer_started")

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

	dbClient.Close()                                          // Close the pool only after the drain resolves.
	l.Info("Shutdown complete", "event", "shutdown_complete") // Natural return ⇒ exit code 0.
}
