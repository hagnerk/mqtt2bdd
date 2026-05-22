// Package mqtt provides a wrapper around the Eclipse Paho MQTT client library,
// managing connection, subscription, and message reception.
package mqtt

import "time"

// MessageHandler is a callback function invoked for each MQTT message received.
// topic is the full MQTT topic string. payload is the raw message bytes (typically JSON).
type MessageHandler func(topic string, payload []byte)

// Message holds a single MQTT message received from the broker.
// It is produced by the MQTT message handler and consumed by the DB writer goroutine.
type Message struct {
	Topic     string
	Timestamp time.Time
	Payload   []byte // Raw JSON as received from the broker
}
