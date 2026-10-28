package middleware

import (
	"context"
	"sync"

	"github.com/doug-benn/mqtt-logger/handlers"
)

// Changed is the change gate (distinctUntilChanged): a message passes only
// when its key differs from the key of the last message that passed — a
// later recurrence of an old value passes again, so this is not a
// seen-set. The gate keeps one state per chain and is mutex-guarded
// because a chain runs on many pool goroutines at once.
func Changed[T any, K comparable](key func(T) K) Middleware[T] {
	var (
		mu   sync.Mutex
		last K
		set  bool
	)
	return func(next handlers.Handler[T]) handlers.Handler[T] {
		return func(ctx context.Context, msg T) {
			k := key(msg)
			mu.Lock()
			// The first message always passes: without this, an initial
			// zero-valued key would match last's zero value and drop.
			pass := !set || k != last
			if pass {
				last, set = k, true
			}
			mu.Unlock()
			if !pass {
				return
			}
			next(ctx, msg)
		}
	}
}
