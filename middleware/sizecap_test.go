package middleware

import (
	"context"
	"testing"

	"github.com/doug-benn/mqtt-logger/handlers"
)

func TestSizeCapDropsOversized(t *testing.T) {
	sizeCap := SizeCap(SizeCapDefault)

	var passed int
	handler := sizeCap(func(ctx context.Context, msg handlers.RawMessage) { passed++ })

	handler(context.Background(), handlers.RawMessage{Topic: "rainfall", Payload: make([]byte, SizeCapDefault)})
	if passed != 1 {
		t.Fatalf("at-cap message passed = %d, want 1", passed)
	}
	handler(context.Background(), handlers.RawMessage{Topic: "rainfall", Payload: make([]byte, SizeCapDefault+1)})
	if passed != 1 {
		t.Fatalf("oversized message reached the handler, passed = %d", passed)
	}
}
