// Package mqtt provides a wrapper around the Eclipse Paho MQTT client library,
// managing connection, subscription, and message reception.
package mqtt

import (
	"context"
	"fmt"
	"log/slog"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/spydemon/mqtt2bdd/internal/config"
)

const qos0 = byte(0)

// Client wraps the Eclipse Paho MQTT client, providing connection management
// and structured logging.
type Client struct {
	pahoClient mqtt.Client
	brokerURL  string // pre-computed once at construction; avoids retaining cfg credentials
	logger     *slog.Logger
}

// NewClient creates a new MQTT Client configured from cfg, using l for structured
// logging. It does not establish a network connection — call Connect to do so.
func NewClient(cfg *config.Config, l *slog.Logger) *Client {
	brokerURL := fmt.Sprintf("tcp://%s:%d", cfg.MQTTBroker, cfg.MQTTPort)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(brokerURL)
	opts.SetClientID("mqtt2bdd")

	// Optional authentication — only set when credentials are provided.
	if cfg.MQTTUsername != "" {
		opts.SetUsername(cfg.MQTTUsername)
		opts.SetPassword(cfg.MQTTPassword)
	}

	// Automatic reconnection is intentionally disabled here; it will be
	// implemented as a manual reconnection loop in story 2.2.
	opts.SetAutoReconnect(false)

	// Log connection loss; reconnection logic is story 2.2.
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		l.Error("MQTT connection lost", "error", err)
	})

	return &Client{
		pahoClient: mqtt.NewClient(opts),
		brokerURL:  brokerURL,
		logger:     l,
	}
}

// Connect establishes a TCP connection to the MQTT broker. It logs the attempt
// and the outcome. Returns an error if the connection cannot be established.
func (c *Client) Connect(ctx context.Context) error {
	c.logger.Info("connecting to MQTT broker", "broker", c.brokerURL)

	token := c.pahoClient.Connect()
	select {
	case <-token.Done():
	case <-ctx.Done():
		return fmt.Errorf("MQTT connect cancelled for broker %s: %w", c.brokerURL, ctx.Err())
	}
	if err := token.Error(); err != nil {
		c.logger.Error("MQTT connection failed", "broker", c.brokerURL, "error", err)
		return fmt.Errorf("failed to connect to MQTT broker %s: %w", c.brokerURL, err)
	}

	c.logger.Info("MQTT connected", "broker", c.brokerURL)
	return nil
}

// Disconnect cleanly closes the connection to the MQTT broker, waiting up to
// 250 milliseconds for any in-flight operations to complete.
func (c *Client) Disconnect() {
	const quiesceMs = 250
	c.pahoClient.Disconnect(quiesceMs)
	c.logger.Info("MQTT disconnected")
}

// IsConnected reports whether the client currently has an active connection
// to the MQTT broker.
func (c *Client) IsConnected() bool {
	return c.pahoClient.IsConnected()
}

// Subscribe registers handler to be called for each message received on topic.
// It uses QoS 0 (at-most-once delivery). The wildcard topic "#" captures all
// published topics. The handler is invoked in a goroutine managed by the Paho
// library — Subscribe returns immediately after the broker acknowledges the
// subscription.
func (c *Client) Subscribe(topic string, handler MessageHandler) error {
	c.logger.Info("subscribing to MQTT topic", "topic", topic)
	token := c.pahoClient.Subscribe(topic, qos0, func(_ mqtt.Client, msg mqtt.Message) {
		handler(msg.Topic(), msg.Payload())
	})
	<-token.Done()
	if err := token.Error(); err != nil {
		c.logger.Error("MQTT subscription failed", "topic", topic, "error", err)
		return fmt.Errorf("failed to subscribe to topic %s: %w", topic, err)
	}
	c.logger.Info("MQTT subscribed", "topic", topic)
	return nil
}
