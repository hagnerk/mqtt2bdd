// Package config handles loading and validating application configuration
// from environment variables following twelve-factor app principles.
package config

import (
	"fmt"
	"os"
	"strconv"
)

const (
	defaultLogLevel   = "INFO"
	defaultBufferSize = 1000
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// MQTT Configuration
	MQTTBroker   string
	MQTTPort     int
	MQTTUsername string // Optional — default ""
	MQTTPassword string // Optional — default ""

	// PostgreSQL Configuration
	PostgresHost     string
	PostgresPort     int
	PostgresDB       string
	PostgresUser     string
	PostgresPassword string

	// Application Configuration
	LogLevel   string // Default "INFO"
	BufferSize int    // Default 1000 — message channel capacity
}

// LoadConfig reads application configuration from environment variables.
// Returns an error if any required variable is missing or invalid.
func LoadConfig() (*Config, error) {
	cfg := &Config{}

	// Required string variables
	required := []struct {
		field *string
		name  string
	}{
		{&cfg.MQTTBroker, "MQTT_BROKER"},
		{&cfg.PostgresHost, "POSTGRES_HOST"},
		{&cfg.PostgresDB, "POSTGRES_DB"},
		{&cfg.PostgresUser, "POSTGRES_USER"},
		{&cfg.PostgresPassword, "POSTGRES_PASSWORD"},
	}

	for _, r := range required {
		val := os.Getenv(r.name)
		if val == "" {
			return nil, fmt.Errorf("required environment variable %s is not set", r.name)
		}
		*r.field = val
	}

	// Required integer variables
	mqttPortStr := os.Getenv("MQTT_PORT")
	if mqttPortStr == "" {
		return nil, fmt.Errorf("required environment variable MQTT_PORT is not set")
	}
	mqttPort, err := strconv.Atoi(mqttPortStr)
	if err != nil {
		return nil, fmt.Errorf("MQTT_PORT must be a valid integer: %w", err)
	}
	cfg.MQTTPort = mqttPort

	postgresPortStr := os.Getenv("POSTGRES_PORT")
	if postgresPortStr == "" {
		return nil, fmt.Errorf("required environment variable POSTGRES_PORT is not set")
	}
	postgresPort, err := strconv.Atoi(postgresPortStr)
	if err != nil {
		return nil, fmt.Errorf("POSTGRES_PORT must be a valid integer: %w", err)
	}
	cfg.PostgresPort = postgresPort

	// Optional variables with defaults
	cfg.MQTTUsername = os.Getenv("MQTT_USERNAME")
	cfg.MQTTPassword = os.Getenv("MQTT_PASSWORD")

	if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
		cfg.LogLevel = logLevel
	} else {
		cfg.LogLevel = defaultLogLevel
	}

	if bufferSizeStr := os.Getenv("BUFFER_SIZE"); bufferSizeStr != "" {
		bufferSize, err := strconv.Atoi(bufferSizeStr)
		if err != nil {
			return nil, fmt.Errorf("BUFFER_SIZE must be a valid integer: %w", err)
		}
		cfg.BufferSize = bufferSize
	} else {
		cfg.BufferSize = defaultBufferSize
	}

	return cfg, nil
}
