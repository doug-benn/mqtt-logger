package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/doug-benn/mqtt-logger/batch"
	"github.com/doug-benn/mqtt-logger/model"
)

type fakeMessage struct {
	topic   string
	payload []byte
}

func (f fakeMessage) Duplicate() bool   { return false }
func (f fakeMessage) Qos() byte         { return 0 }
func (f fakeMessage) Retained() bool    { return false }
func (f fakeMessage) Topic() string     { return f.topic }
func (f fakeMessage) Payload() []byte   { return f.payload }
func (f fakeMessage) Ack()              {}
func (f fakeMessage) MessageID() uint16 { return 0 }

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

	handler(context.Background(), fakeMessage{topic: "rainfall", payload: []byte(`{"rainfall": 1.5}`)})

	if len(repo.created) != 1 || repo.created[0] != 1.5 {
		t.Fatalf("created = %v, want [1.5]", repo.created)
	}
}

func TestRainfallHandlerDropsMalformed(t *testing.T) {
	repo := &fakeRainfallRepo{}
	handler := RainfallHandler(repo)

	handler(context.Background(), fakeMessage{topic: "rainfall", payload: []byte("not json")})
	handler(context.Background(), fakeMessage{topic: "rainfall", payload: []byte(`{}`)})

	if len(repo.created) != 0 {
		t.Fatalf("malformed payloads reached the repository: %v", repo.created)
	}
}

func TestRainfallHandlerDropsOnDatabaseError(t *testing.T) {
	repo := &fakeRainfallRepo{err: context.DeadlineExceeded}
	handler := RainfallHandler(repo)

	// The drop is logged, not propagated — a DB failure must not panic the
	// dispatch goroutine.
	handler(context.Background(), fakeMessage{topic: "rainfall", payload: []byte(`{"rainfall": 2}`)})
}

func TestTemperatureHandlerLegacyBehavior(t *testing.T) {
	var got []model.TemperatureMessage
	tempBatch := batch.NewBatchProcessor(10, time.Hour, func(items []model.TemperatureMessage) error {
		got = append(got, items...)
		return nil
	})
	handler := TemperatureHandler(tempBatch)

	handler(context.Background(), fakeMessage{topic: "temperature", payload: []byte("23.5")})
	// Legacy decode is preserved byte-for-byte: a malformed payload still
	// queues a zero-value reading rather than being dropped.
	handler(context.Background(), fakeMessage{topic: "temperature", payload: []byte("not-a-number")})

	if err := tempBatch.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("batched = %v, want two readings", got)
	}
	if got[0].Temperature != 23.5 {
		t.Errorf("first reading = %v, want 23.5", got[0].Temperature)
	}
	if got[1].Temperature != 0 {
		t.Errorf("legacy malformed reading = %v, want zero-fill", got[1].Temperature)
	}
}
