package middleware

import (
	"context"
	"fmt"

	"github.com/doug-benn/mqtt-logger/handlers"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// SizeCapDefault is the raw-payload cap applied when no other size is
// given.
const SizeCapDefault = 1024

// SizeCap drops raw payloads larger than maxBytes. It operates at the
// raw-message stage, so unlike the other stages it is bound to
// mqtt.Message — payload access is what it needs. Over-cap drops are
// logged: they are the drop a site operator can actually act on.
func SizeCap(maxBytes int) Middleware[mqtt.Message] {
	return func(next handlers.Handler[mqtt.Message]) handlers.Handler[mqtt.Message] {
		return func(ctx context.Context, msg mqtt.Message) {
			if len(msg.Payload()) > maxBytes {
				fmt.Printf("Dropping oversized message from %s: %d bytes > cap %d\n",
					msg.Topic(), len(msg.Payload()), maxBytes)
				return
			}
			next(ctx, msg)
		}
	}
}
