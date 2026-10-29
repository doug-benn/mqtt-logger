package handlers

import "context"

// RawMessage is the raw message: topic plus still-undecoded payload. It is
// the carrier at the raw-message stage of a chain — the MQTT library is
// adapted to it exactly once, at the topic route, and never seen past it.
type RawMessage struct {
	Topic   string
	Payload []byte
}

// Handler processes one message of type T. It is the signature every topic
// handler is built from and the value middleware wraps.
type Handler[T any] func(ctx context.Context, msg T)
