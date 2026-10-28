package middleware

import (
	"context"
	"testing"

	"github.com/doug-benn/mqtt-logger/handlers"
)

type fakeMessage struct{ payload string }

// passRecorder counts messages that reached the handler behind a stage.
type passRecorder struct{ passed []int }

func (r *passRecorder) handler() handlers.Handler[int] {
	return func(ctx context.Context, msg int) { r.passed = append(r.passed, msg) }
}

func TestChainFirstMiddlewareRunsOutermost(t *testing.T) {
	var log []string
	record := func(name string) Middleware[fakeMessage] {
		return func(next handlers.Handler[fakeMessage]) handlers.Handler[fakeMessage] {
			return func(ctx context.Context, msg fakeMessage) {
				log = append(log, name+"-in")
				next(ctx, msg)
				log = append(log, name+"-out")
			}
		}
	}

	handler := Chain(
		func(ctx context.Context, msg fakeMessage) { log = append(log, "handler") },
		record("a"),
		record("b"),
	)

	handler(context.Background(), fakeMessage{})

	want := []string{"a-in", "b-in", "handler", "b-out", "a-out"}
	if len(log) != len(want) {
		t.Fatalf("log = %v, want %v", log, want)
	}
	for i := range want {
		if log[i] != want[i] {
			t.Fatalf("log = %v, want %v", log, want)
		}
	}
}
