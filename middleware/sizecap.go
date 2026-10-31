package middleware

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/doug-benn/mqtt-logger/handlers"
)

// SizeCapDefault is the raw-payload cap applied when no other size is
// given.
const SizeCapDefault = 1024

// SizeCap drops raw messages whose payload is larger than maxBytes. It
// operates at the raw-message stage, so it sees the raw carrier rather than
// a decoded reading. Over-cap drops are logged: they are the drop a site
// operator can actually act on.
func SizeCap(logger *slog.Logger, maxBytes int) Middleware[handlers.RawMessage] {
	return func(next handlers.Handler[handlers.RawMessage]) handlers.Handler[handlers.RawMessage] {
		return func(ctx context.Context, msg handlers.RawMessage) {
			if len(msg.Payload) > maxBytes {
				logger.Warn(fmt.Sprintf("dropping oversized message: %d bytes > cap %d",
					len(msg.Payload), maxBytes), "topic", msg.Topic)
				return
			}
			next(ctx, msg)
		}
	}
}
