package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/spydemon/mqtt2bdd/internal/config"
	"github.com/spydemon/mqtt2bdd/internal/database"
	"github.com/spydemon/mqtt2bdd/internal/logger"
	"github.com/spydemon/mqtt2bdd/internal/mqtt"
)

// defaultRetryIntervalOnDatabaseFailure is the delay between insert retries in the DB writer goroutine.
const defaultRetryIntervalOnDatabaseFailure = 10 * time.Second

// Version is injected at build time via ldflags. Defaults to "dev" for local builds.
var Version = "dev"

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	l := logger.InitLogger(cfg.LogLevel)
	l.Info("MQTT2BDD starting", "version", Version)

	mqttClient := mqtt.NewClient(cfg, l)
	if err := mqttClient.Connect(context.Background()); err != nil {
		os.Exit(1)
	}
	defer mqttClient.Disconnect()

	dbClient := database.NewClient(cfg, l)
	if err := dbClient.Connect(context.Background()); err != nil {
		os.Exit(1)
	}
	defer dbClient.Close()

	// msgChan decouples MQTT message reception from database writes (Go CSP pattern).
	// Producer: MQTT message handler (Paho callback goroutine).
	// Consumer: DB writer goroutine below.
	msgChan := make(chan mqtt.Message, cfg.BufferSize)

	// DB writer goroutine: sole consumer of msgChan, exits when channel closed (story 2.4).
	// On write failure, the message is retained and retried until success so no message is
	// lost during a database outage. Story 2.3 will add detailed retry logging.
	go func() {
		for msg := range msgChan {
			for {
				err := dbClient.InsertMessage(context.Background(), msg.Topic, msg.Timestamp, json.RawMessage(msg.Payload))
				if err == nil {
					break
				}
				// error already logged by InsertMessage; hold this message and retry after delay
				time.Sleep(defaultRetryIntervalOnDatabaseFailure)
			}
		}
	}()

	mqttMessageHandler := func(topic string, payload []byte) {
		msg := mqtt.Message{Topic: topic, Timestamp: time.Now(), Payload: payload}
		select {
		case msgChan <- msg:
		default:
			l.Warn("message buffer full, blocking until space available", "buffer_size", cfg.BufferSize)
			msgChan <- msg // block until DB writer consumes a slot
		}
	}
	if err := mqttClient.Subscribe("#", mqttMessageHandler); err != nil {
		l.Error("MQTT subscribe failed", "error", err)
		os.Exit(1)
	}

	select {} // Block until process is killed — signal handling added in story 2.4
}
