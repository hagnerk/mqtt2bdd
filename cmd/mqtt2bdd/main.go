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

	dbClient := database.NewClient(cfg, l)
	if err := dbClient.Connect(context.Background()); err != nil {
		os.Exit(1)
	}
	defer dbClient.Close()

	mqttMessageHandler := func(topic string, payload []byte) {
		go func() {
			if err := dbClient.InsertMessage(context.Background(), topic, time.Now(), json.RawMessage(payload)); err != nil {
				return // error already logged by InsertMessage
			}
		}()
	}
	if err := mqttClient.Subscribe("#", mqttMessageHandler); err != nil {
		l.Error("MQTT subscribe failed", "error", err)
		os.Exit(1)
	}

	select {} // Block until process is killed — signal handling added in story 2.x
}
