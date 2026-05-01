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

	mqttMessageHandler := func(topic string, payload []byte) {
		preview := payload
		if len(preview) > 100 {
			preview = preview[:100]
		}
		l.Debug("message received", "topic", topic, "payload_length", len(payload), "payload_preview", string(preview))
	}
	if err := mqttClient.Subscribe("#", mqttMessageHandler); err != nil {
		l.Error("MQTT subscribe failed", "error", err)
		os.Exit(1)
	}

	dbClient := database.NewClient(cfg, l)
	if err := dbClient.Connect(context.Background()); err != nil {
		os.Exit(1)
	}
	defer dbClient.Close()

	if err := dbClient.InsertMessage(context.Background(), "test/sensor", time.Now(), json.RawMessage(`{"temperature": 21.3}`)); err != nil {
		l.Error("test insert failed", "error", err)
	}

	select {} // Block until process is killed — signal handling added in story 2.x
}
