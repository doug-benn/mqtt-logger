package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/doug-benn/mqtt-logger/handlers"
	"github.com/doug-benn/mqtt-logger/middleware"
	"github.com/doug-benn/mqtt-logger/model"
	"github.com/doug-benn/mqtt-logger/repository"
)

// fakeStore is a partial fake: the embedded nil Store satisfies every method
// the seam has today, and an un-overridden call panics only if a test actually
// exercises it — so the fake stops growing whenever a message adds a method.
type fakeStore struct {
	repository.Store
	created  []float64
	tempRows []model.Temperature
	humRows  []model.Humidity
}

func (f *fakeStore) CreateRainfall(ctx context.Context, value float64) (model.Rainfall, error) {
	f.created = append(f.created, value)
	return model.Rainfall{Value: value}, nil
}

func (f *fakeStore) CreateTemperatureBatch(ctx context.Context, batch []model.Temperature) error {
	f.tempRows = append(f.tempRows, batch...)
	return nil
}

func (f *fakeStore) CreateHumidities(ctx context.Context, batch []model.Humidity) error {
	f.humRows = append(f.humRows, batch...)
	return nil
}

// discardLogger silences logging in wiring tests: they assert behavior, not
// log output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// routesFixture builds the route set with a test double in place of the
// store, so tests cross the composed chains exactly as runMQTT wires them.
// The returned flush closes every buffering chain, flushing queued readings
// into the store's recorded rows before assertions run.
func routesFixture(t *testing.T) (routes map[string]handlers.Handler[handlers.RawMessage], repo *fakeStore, flush func()) {
	t.Helper()

	repo = &fakeStore{}
	routes, closeBuffers := buildRoutes(repo, discardLogger())
	return routes, repo, closeBuffers
}

func TestSubscribeTopicsDerivesFromRoutes(t *testing.T) {
	routes, _, _ := routesFixture(t)

	topics := subscribeTopics(routes)

	if len(topics) != len(routes) {
		t.Fatalf("topics = %v, want one per route (%d)", topics, len(routes))
	}
	for _, topic := range []string{"rainfall", "temperature"} {
		if qos, ok := topics[topic]; !ok || qos != 0 {
			t.Errorf("topics[%q] = %d, %v; want QoS 0 present", topic, qos, ok)
		}
	}
}

func TestRouteChainPersistsRainfall(t *testing.T) {
	routes, repo, _ := routesFixture(t)

	routes["rainfall"](context.Background(), handlers.RawMessage{
		Topic:   "rainfall",
		Payload: []byte(`{"rainfall": 1.5}`),
	})

	if len(repo.created) != 1 || repo.created[0] != 1.5 {
		t.Fatalf("created = %v, want [1.5]", repo.created)
	}
}

func TestRouteChainDropsOversizedBeforeHandler(t *testing.T) {
	routes, repo, _ := routesFixture(t)

	// Not valid JSON either, but SizeCap runs outermost and must drop it
	// before the handler ever decodes.
	oversized := bytes.Repeat([]byte("x"), middleware.SizeCapDefault+1)
	routes["rainfall"](context.Background(), handlers.RawMessage{
		Topic:   "rainfall",
		Payload: oversized,
	})

	if len(repo.created) != 0 {
		t.Fatalf("oversized payload reached the repository: %v", repo.created)
	}
}

func TestRouteChainFlushesOnlyValidTemperature(t *testing.T) {
	routes, repo, flush := routesFixture(t)

	routes["temperature"](context.Background(), handlers.RawMessage{Topic: "temperature", Payload: []byte("21.5")})
	routes["temperature"](context.Background(), handlers.RawMessage{Topic: "temperature", Payload: []byte("garbage")})
	flush()

	if len(repo.tempRows) != 1 {
		t.Fatalf("flushed rows = %v, want only the valid reading", repo.tempRows)
	}
	if repo.tempRows[0].Value != 21.5 {
		t.Errorf("flushed value = %v, want 21.5", repo.tempRows[0].Value)
	}
	if repo.tempRows[0].ReceivedAt.IsZero() {
		t.Error("flushed ReceivedAt is zero; decode must stamp it")
	}
}

// The humidity route is its declaration exactly as wiring builds it — no
// middleware stages — flushed on close.
func TestRouteChainFlushesHumidity(t *testing.T) {
	routes, repo, flush := routesFixture(t)

	routes["humidity"](context.Background(), handlers.RawMessage{Topic: "humidity", Payload: []byte("48.2")})
	routes["humidity"](context.Background(), handlers.RawMessage{Topic: "humidity", Payload: []byte("garbage")})
	flush()

	if len(repo.humRows) != 1 {
		t.Fatalf("flushed humidity rows = %v, want only the valid reading", repo.humRows)
	}
	if repo.humRows[0].Value != 48.2 {
		t.Errorf("flushed value = %v, want 48.2", repo.humRows[0].Value)
	}
}

// The rainfall chain's limiter, proven at the route: a burst beyond the 1s
// token budget persists exactly one reading — the wired default is 1/s with
// burst 1, and drops at the limiter are silent by decision.
func TestRouteChainRateLimitsBurst(t *testing.T) {
	routes, repo, _ := routesFixture(t)

	for _, payload := range []string{
		`{"rainfall": 1}`, `{"rainfall": 2}`, `{"rainfall": 3}`,
		`{"rainfall": 4}`, `{"rainfall": 5}`,
	} {
		routes["rainfall"](context.Background(), handlers.RawMessage{Topic: "rainfall", Payload: []byte(payload)})
	}

	if len(repo.created) != 1 {
		t.Fatalf("burst persisted %v, want exactly one row (rate limit at 1/s, burst 1)", repo.created)
	}
}
