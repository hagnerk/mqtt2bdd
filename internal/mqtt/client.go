// Package mqtt provides a wrapper around the Eclipse Paho MQTT client library,
// managing connection, subscription, and message reception.
package mqtt

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/spydemon/mqtt2bdd/internal/config"
)

const (
	qos0                         = byte(0)
	defaultMQTTReconnectInterval = 10 * time.Second
)

// Client wraps the Eclipse Paho MQTT client, providing connection management
// and structured logging.
type Client struct {
	pahoClient        mqtt.Client
	brokerURL         string // pre-computed once at construction; avoids retaining cfg credentials
	logger            *slog.Logger
	subscribedTopic   string
	subscribedHandler MessageHandler
}

// NewClient creates a new MQTT Client configured from cfg, using l for structured
// logging. It does not establish a network connection — call Connect to do so.
func NewClient(cfg *config.Config, l *slog.Logger) *Client {
	brokerURL := fmt.Sprintf("tcp://%s:%d", cfg.MQTTBroker, cfg.MQTTPort)

	// c is initialised before opts so the connection-lost closure can capture it.
	c := &Client{
		brokerURL: brokerURL,
		logger:    l,
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(brokerURL)
	opts.SetClientID("mqtt2bdd")

	// Optional authentication — only set when credentials are provided.
	if cfg.MQTTUsername != "" {
		opts.SetUsername(cfg.MQTTUsername)
		opts.SetPassword(cfg.MQTTPassword)
	}

	opts.SetAutoReconnect(false)

	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		c.logger.Error("MQTT connection lost", "error", err)
		go c.reconnectLoop()
	})

	c.pahoClient = mqtt.NewClient(opts)
	return c
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
	c.subscribedTopic = topic
	c.subscribedHandler = handler

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

// reconnectLoop retries connecting to the MQTT broker every defaultMQTTReconnectInterval
// until a connection is re-established, then re-subscribes to the last registered topic.
// Launched as a goroutine by the connection-lost handler.
func (c *Client) reconnectLoop() {
	for attempt := 1; ; attempt++ {
		time.Sleep(defaultMQTTReconnectInterval)
		c.logger.Info("Attempting MQTT reconnection", "attempt", attempt)

		token := c.pahoClient.Connect()
		<-token.Done()
		if err := token.Error(); err != nil {
			c.logger.Error("MQTT reconnection failed", "attempt", attempt, "error", err)
			continue
		}

		c.logger.Info("MQTT reconnected successfully")
		if c.subscribedHandler != nil {
			if err := c.Subscribe(c.subscribedTopic, c.subscribedHandler); err != nil {
				c.logger.Error("MQTT re-subscription failed after reconnect", "error", err)
			}
		}
		return
	}
}
