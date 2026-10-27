package handlers

import "context"

// Handler processes one message of type T. It is the signature every topic
// handler is built from and the value middleware wraps.
type Handler[T any] func(ctx context.Context, msg T)
