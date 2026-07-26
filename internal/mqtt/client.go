// Package mqtt provides a wrapper around the Eclipse Paho MQTT client library,
// managing connection, subscription, and message reception.
package mqtt

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/spydemon/mqtt2bdd/internal/config"
	"github.com/spydemon/mqtt2bdd/internal/logger"
)

const (
	qos0 = byte(0)
)

// defaultMQTTReconnectInterval is the cadence of both the connect retry and the
// re-subscribe retry in reconnectLoop. A var, not a const: client_test.go shrinks
// it for the duration of its own test so the retry-until-success unit test does not
// have to wait out a real 10 s interval per attempt.
var defaultMQTTReconnectInterval = 10 * time.Second

// Client wraps the Eclipse Paho MQTT client, providing connection management
// and structured logging.
type Client struct {
	pahoClient        mqtt.Client
	brokerURL         string // pre-computed once at construction; avoids retaining cfg credentials
	logger            *slog.Logger
	subscribedTopic   string
	subscribedHandler MessageHandler
	shutdown          chan struct{} // closed by Disconnect to stop a running reconnectLoop
	closeOnce         sync.Once     // guards closing shutdown exactly once
}

// NewClient creates a new MQTT Client configured from cfg, using l for structured
// logging. It does not establish a network connection — call Connect to do so.
func NewClient(cfg *config.Config, l *slog.Logger) *Client {
	brokerURL := fmt.Sprintf("tcp://%s:%d", cfg.MQTTBroker, cfg.MQTTPort)

	// c is initialised before opts so the connection-lost closure can capture it.
	// The component and broker context is bound once here, so no call site repeats it.
	c := &Client{
		brokerURL: brokerURL,
		logger:    l.With("component", logger.ComponentMQTT, "broker", brokerURL),
		shutdown:  make(chan struct{}),
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
		// No operation attribute: the broker dropped the link on its own, so this
		// reports a state rather than a failed local attempt.
		c.logger.Error("MQTT connection lost", "event", "connection_lost", "error", err)
		go c.reconnectLoop()
	})

	c.pahoClient = mqtt.NewClient(opts)
	return c
}

// Connect establishes a TCP connection to the MQTT broker. It logs the attempt
// and the outcome. Returns an error if the connection cannot be established.
func (c *Client) Connect(ctx context.Context) error {
	c.logger.Info("connecting to MQTT broker", "event", "connecting")

	token := c.pahoClient.Connect()
	select {
	case <-token.Done():
	case <-ctx.Done():
		// ctx.Err() rather than a fixed string, so a future non-deadline cancellation
		// stays distinguishable from the connect timeout that is its only trigger today.
		c.logger.Error("MQTT connect timed out", "event", "connect_timeout", "operation", "connect", "error", ctx.Err())
		return fmt.Errorf("MQTT connect cancelled for broker %s: %w", c.brokerURL, ctx.Err())
	}
	if err := token.Error(); err != nil {
		c.logger.Error("MQTT connection failed", "event", "connect_failed", "operation", "connect", "error", err)
		return fmt.Errorf("failed to connect to MQTT broker %s: %w", c.brokerURL, err)
	}

	c.logger.Info("MQTT connected", "event", "connected")
	return nil
}

// Disconnect cleanly closes the connection to the MQTT broker, waiting up to
// 250 milliseconds for any in-flight operations to complete.
func (c *Client) Disconnect() {
	const quiesceMs = 250
	// Signal any running reconnectLoop to stop before tearing down the connection.
	c.closeOnce.Do(func() { close(c.shutdown) })
	c.pahoClient.Disconnect(quiesceMs)
	c.logger.Info("MQTT disconnected", "event", "disconnected")
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

	c.logger.Info("subscribing to MQTT topic", "event", "subscribing", "topic", topic)
	token := c.pahoClient.Subscribe(topic, qos0, func(_ mqtt.Client, msg mqtt.Message) {
		// Level-guarded because this is the per-message hot path: slog evaluates its
		// variadic arguments before checking the level, so the payload conversion
		// would copy every message in full even when running at INFO.
		if c.logger.Enabled(context.Background(), slog.LevelDebug) {
			// string(...) not the raw []byte: TextHandler renders a byte slice as
			// decimal values ("[123 34 ...]"), which is unreadable.
			c.logger.Debug("MQTT message received",
				"event", "message_received",
				"topic", msg.Topic(),
				"payload_size", len(msg.Payload()),
				"payload", string(msg.Payload()),
			)
		}
		handler(msg.Topic(), msg.Payload())
	})
	<-token.Done()
	if err := token.Error(); err != nil {
		c.logger.Error("MQTT subscription failed", "event", "subscribe_failed", "operation", "subscribe", "topic", topic, "error", err)
		return fmt.Errorf("failed to subscribe to topic %s: %w", topic, err)
	}
	c.logger.Info("MQTT subscribed", "event", "subscribed", "topic", topic)
	return nil
}

// reconnectLoop retries connecting to the MQTT broker every defaultMQTTReconnectInterval
// until a connection is re-established, then re-subscribes to the last registered topic.
// Launched as a goroutine by the connection-lost handler.
func (c *Client) reconnectLoop() {
	for attempt := 1; ; attempt++ {
		select {
		case <-time.After(defaultMQTTReconnectInterval):
		case <-c.shutdown:
			c.logger.Info("MQTT reconnect cancelled, shutting down", "event", "reconnect_cancelled")
			return
		}
		c.logger.Info("Attempting MQTT reconnection", "event", "reconnecting", "attempt", attempt)

		token := c.pahoClient.Connect()
		<-token.Done()
		if err := token.Error(); err != nil {
			c.logger.Error("MQTT reconnection failed", "event", "reconnect_failed", "operation", "reconnect", "attempt", attempt, "error", err)
			continue
		}

		c.logger.Info("MQTT reconnected successfully", "event", "reconnected")
		if c.subscribedHandler != nil {
			for {
				err := c.Subscribe(c.subscribedTopic, c.subscribedHandler)
				if err == nil {
					break
				}
				// Subscribe() itself already logged the generic subscribe_failed (above).
				// This entry is the distinct, documented resubscribe_failed event — the one
				// operators filter on to see specifically "a reconnect's subscribe attempt
				// failed" — now firing on EVERY failed attempt, not just the first.
				c.logger.Error("MQTT re-subscription failed after reconnect", "event", "resubscribe_failed", "operation", "subscribe", "error", err)
				select {
				case <-time.After(defaultMQTTReconnectInterval):
				case <-c.shutdown:
					c.logger.Info("MQTT reconnect cancelled, shutting down", "event", "reconnect_cancelled")
					return
				}
			}
		}
		return
	}
}
