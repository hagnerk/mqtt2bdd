package main

import (
	"context"
	"log"
	"os"

	"github.com/spydemon/mqtt2bdd/internal/config"
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
}
