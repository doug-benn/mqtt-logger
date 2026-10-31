package middleware

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/doug-benn/mqtt-logger/handlers"
)

func TestSizeCapDropsOversized(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sizeCap := SizeCap(logger, SizeCapDefault)

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
