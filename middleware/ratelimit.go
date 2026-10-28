package middleware

import (
	"context"

	"github.com/doug-benn/mqtt-logger/handlers"
	"golang.org/x/time/rate"
)

// DefaultRate is the wired message rate for a chain's limiter.
const DefaultRate = rate.Limit(1)

// DefaultBurst is the token-bucket burst for a chain's limiter.
const DefaultBurst = 1

// RateLimit drops messages when the shared token bucket has no token left.
// The limiter is constructed per chain; the wiring sets the rate.
func RateLimit[T any](limiter *rate.Limiter) Middleware[T] {
	return func(next handlers.Handler[T]) handlers.Handler[T] {
		return func(ctx context.Context, msg T) {
			if !limiter.Allow() {
				return
			}
			next(ctx, msg)
		}
	}
}
