package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/doug-benn/mqtt-logger/batch"
	"github.com/doug-benn/mqtt-logger/handlers"
	"github.com/doug-benn/mqtt-logger/middleware"
	"github.com/doug-benn/mqtt-logger/model"
)

type fakeRainfallRepo struct {
	created []float64
}

func (f *fakeRainfallRepo) Create(ctx context.Context, value float64) (model.Rainfall, error) {
	f.created = append(f.created, value)
	return model.Rainfall{Value: value}, nil
}

func (f *fakeRainfallRepo) List(ctx context.Context) ([]model.Rainfall, error) {
	return nil, nil
}

// routesFixture builds the route set with test doubles in place of the real
// dependencies, so tests cross the composed chains exactly as runMQTT wires
// them. The returned flush closes the temperature batch, flushing queued
// readings into temps before assertions run.
func routesFixture(t *testing.T) (routes map[string]handlers.Handler[handlers.RawMessage], repo *fakeRainfallRepo, temps *[]model.TemperatureMessage, flush func()) {
	t.Helper()

	repo = &fakeRainfallRepo{}
	var captured []model.TemperatureMessage
	tempBatch := batch.NewBatchProcessor(10, time.Hour, func(items []model.TemperatureMessage) error {
		captured = append(captured, items...)
		return nil
	})
	temps = &captured

	return buildRoutes(repo, tempBatch), repo, temps, func() {
		if err := tempBatch.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}
}

func TestSubscribeTopicsDerivesFromRoutes(t *testing.T) {
	routes, _, _, _ := routesFixture(t)

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
	routes, repo, _, _ := routesFixture(t)

	routes["rainfall"](context.Background(), handlers.RawMessage{
		Topic:   "rainfall",
		Payload: []byte(`{"rainfall": 1.5}`),
	})

	if len(repo.created) != 1 || repo.created[0] != 1.5 {
		t.Fatalf("created = %v, want [1.5]", repo.created)
	}
}

func TestRouteChainDropsOversizedBeforeHandler(t *testing.T) {
	routes, repo, _, _ := routesFixture(t)

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

func TestRouteChainQueuesOnlyValidTemperature(t *testing.T) {
	routes, _, temps, flush := routesFixture(t)

	routes["temperature"](context.Background(), handlers.RawMessage{Topic: "temperature", Payload: []byte("21.5")})
	routes["temperature"](context.Background(), handlers.RawMessage{Topic: "temperature", Payload: []byte("garbage")})
	flush()

	if len(*temps) != 1 {
		t.Fatalf("queued = %v, want only the valid reading", *temps)
	}
	if (*temps)[0].Temperature != 21.5 {
		t.Errorf("queued = %v, want 21.5", (*temps)[0].Temperature)
	}
}
