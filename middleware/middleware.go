// Package middleware provides composable middleware for MQTT message
// processing: Middleware[T] wraps a handlers.Handler[T] the way net/http
// middleware wraps an http.Handler.
package middleware

import (
	"github.com/doug-benn/mqtt-logger/handlers"
)

// Middleware wraps a Handler with behavior before and/or after it.
type Middleware[T any] func(next handlers.Handler[T]) handlers.Handler[T]

// Chain composes middlewares around handler. The first middleware runs
// outermost — it sees the message first.
func Chain[T any](handler handlers.Handler[T], mw ...Middleware[T]) handlers.Handler[T] {
	for i := len(mw) - 1; i >= 0; i-- {
		handler = mw[i](handler)
	}
	return handler
}
