package middleware

import (
	"context"
	"fmt"

	"github.com/doug-benn/mqtt-logger/handlers"
)

// SizeCapDefault is the raw-payload cap applied when no other size is
// given.
const SizeCapDefault = 1024

// SizeCap drops raw messages whose payload is larger than maxBytes. It
// operates at the raw-message stage, so it sees the raw carrier rather than
// a decoded reading. Over-cap drops are logged: they are the drop a site
// operator can actually act on.
func SizeCap(maxBytes int) Middleware[handlers.RawMessage] {
	return func(next handlers.Handler[handlers.RawMessage]) handlers.Handler[handlers.RawMessage] {
		return func(ctx context.Context, msg handlers.RawMessage) {
			if len(msg.Payload) > maxBytes {
				fmt.Printf("Dropping oversized message from %s: %d bytes > cap %d\n",
					msg.Topic, len(msg.Payload), maxBytes)
				return
			}
			next(ctx, msg)
		}
	}
}
