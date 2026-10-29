package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/doug-benn/mqtt-logger/batch"
	"github.com/doug-benn/mqtt-logger/model"
)

type fakeRainfallRepo struct {
	created []float64
	err     error
}

func (f *fakeRainfallRepo) Create(ctx context.Context, value float64) (model.Rainfall, error) {
	f.created = append(f.created, value)
	return model.Rainfall{Value: value}, f.err
}

func (f *fakeRainfallRepo) List(ctx context.Context) ([]model.Rainfall, error) {
	return nil, nil
}

func TestRainfallHandlerPersists(t *testing.T) {
	repo := &fakeRainfallRepo{}
	handler := RainfallHandler(repo)

	handler(context.Background(), RawMessage{Topic: "rainfall", Payload: []byte(`{"rainfall": 1.5}`)})

	if len(repo.created) != 1 || repo.created[0] != 1.5 {
		t.Fatalf("created = %v, want [1.5]", repo.created)
	}
}

func TestRainfallHandlerDropsMalformed(t *testing.T) {
	repo := &fakeRainfallRepo{}
	handler := RainfallHandler(repo)

	handler(context.Background(), RawMessage{Topic: "rainfall", Payload: []byte("not json")})
	handler(context.Background(), RawMessage{Topic: "rainfall", Payload: []byte(`{}`)})

	if len(repo.created) != 0 {
		t.Fatalf("malformed payloads reached the repository: %v", repo.created)
	}
}

func TestRainfallHandlerDropsOnDatabaseError(t *testing.T) {
	repo := &fakeRainfallRepo{err: context.DeadlineExceeded}
	handler := RainfallHandler(repo)

	// The drop is logged, not propagated — a DB failure must not panic the
	// dispatch goroutine.
	handler(context.Background(), RawMessage{Topic: "rainfall", Payload: []byte(`{"rainfall": 2}`)})
}

func TestTemperatureHandlerDropsMalformed(t *testing.T) {
	var got []model.TemperatureMessage
	tempBatch := batch.NewBatchProcessor(10, time.Hour, func(items []model.TemperatureMessage) error {
		got = append(got, items...)
		return nil
	})
	handler := TemperatureHandler(tempBatch)

	handler(context.Background(), RawMessage{Topic: "temperature", Payload: []byte("23.5")})
	// A malformed payload is dropped in the handler rather than decoded to a
	// zero-value reading — matching the Rainfall handler's semantics.
	handler(context.Background(), RawMessage{Topic: "temperature", Payload: []byte("not-a-number")})

	if err := tempBatch.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("batched = %v, want only the valid reading", got)
	}
	if got[0].Temperature != 23.5 {
		t.Errorf("reading = %v, want 23.5", got[0].Temperature)
	}
}
