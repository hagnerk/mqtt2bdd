package main

import (
	"context"
	"encoding/json"
	"log"
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
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	l := logger.InitLogger(cfg.LogLevel)
	l.Info("MQTT2BDD starting", "version", Version)

	mqttClient := mqtt.NewClient(cfg, l)
	mqttCtx, mqttCancel := context.WithTimeout(context.Background(), defaultConnectTimeout)
	err = mqttClient.Connect(mqttCtx)
	mqttCancel()
	if err != nil {
		os.Exit(1)
	}

	dbClient := database.NewClient(cfg, l)
	dbCtx, dbCancel := context.WithTimeout(context.Background(), defaultConnectTimeout)
	err = dbClient.Connect(dbCtx)
	dbCancel()
	if err != nil {
		os.Exit(1)
	}

	// msgChan decouples MQTT message reception from database writes (Go CSP pattern).
	// Producer: MQTT message handler (Paho callback goroutine).
	// Consumer: DB writer goroutine below.
	msgChan := make(chan mqtt.Message, cfg.BufferSize)

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

	mqttMessageHandler := func(topic string, payload []byte) {
		msg := mqtt.Message{Topic: topic, Timestamp: time.Now(), Payload: payload}
		select {
		case msgChan <- msg:
		default:
			l.Warn("message buffer full, blocking until space available", "buffer_size", cfg.BufferSize)
			// Block until the DB writer frees a slot, or shutdown begins (drop the message
			// rather than block on a channel that is draining for the last time).
			select {
			case msgChan <- msg:
			case <-done:
			}
		}
	}
	if err := mqttClient.Subscribe("#", mqttMessageHandler); err != nil {
		l.Error("MQTT subscribe failed", "error", err)
		os.Exit(1)
	}

	// Block until a shutdown signal arrives. Buffered (cap 1) so the signal is not
	// missed if it fires before this goroutine reaches the receive.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	sig := <-sigChan
	l.Info("Shutdown signal received, flushing buffered messages", "signal", sig.String())

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
		l.Info("Flushed buffered messages", "count", buffered)
	case <-time.After(defaultShutdownTimeout):
		l.Warn("Shutdown timeout exceeded; some buffered messages may not have been flushed", "buffered", buffered)
	}

	dbClient.Close()            // Close the pool only after the drain resolves.
	l.Info("Shutdown complete") // Natural return ⇒ exit code 0.
}
