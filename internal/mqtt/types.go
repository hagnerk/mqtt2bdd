// Package mqtt provides a wrapper around the Eclipse Paho MQTT client library,
// managing connection, subscription, and message reception.
package mqtt

// MessageHandler is a callback function invoked for each MQTT message received.
// topic is the full MQTT topic string. payload is the raw message bytes (typically JSON).
type MessageHandler func(topic string, payload []byte)
