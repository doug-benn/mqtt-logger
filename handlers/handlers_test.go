package handlers

import (
	"context"
	"io"
	"log/slog"
	"testing"

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
	tempErr  error
	err      error
}

func (f *fakeStore) CreateRainfall(ctx context.Context, value float64) (model.Rainfall, error) {
	f.created = append(f.created, value)
	return model.Rainfall{Value: value}, f.err
}

func (f *fakeStore) CreateTemperatureBatch(ctx context.Context, batch []model.Temperature) error {
	f.tempRows = append(f.tempRows, batch...)
	return f.tempErr
}

func (f *fakeStore) CreateHumidities(ctx context.Context, batch []model.Humidity) error {
	f.humRows = append(f.humRows, batch...)
	return nil
}

// discardLogger silences drop reporting in tests: they assert behavior, not
// log output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRainfallHandlerPersists(t *testing.T) {
	repo := &fakeStore{}
	handler := RainfallHandler(repo, discardLogger())

	handler(context.Background(), RawMessage{Topic: "rainfall", Payload: []byte(`{"rainfall": 1.5}`)})

	if len(repo.created) != 1 || repo.created[0] != 1.5 {
		t.Fatalf("created = %v, want [1.5]", repo.created)
	}
}

func TestRainfallHandlerDropsMalformed(t *testing.T) {
	repo := &fakeStore{}
	handler := RainfallHandler(repo, discardLogger())

	handler(context.Background(), RawMessage{Topic: "rainfall", Payload: []byte("not json")})
	handler(context.Background(), RawMessage{Topic: "rainfall", Payload: []byte(`{}`)})

	if len(repo.created) != 0 {
		t.Fatalf("malformed payloads reached the repository: %v", repo.created)
	}
}

func TestRainfallHandlerDropsOnDatabaseError(t *testing.T) {
	repo := &fakeStore{err: context.DeadlineExceeded}
	handler := RainfallHandler(repo, discardLogger())

	// The drop is logged, not propagated — a DB failure must not panic the
	// dispatch goroutine.
	handler(context.Background(), RawMessage{Topic: "rainfall", Payload: []byte(`{"rainfall": 2}`)})
}

// TestNewBatchedDropsMalformedAtDecode crosses the seam directly: decode
// failures drop at intake — the buffer and write never see them.
func TestNewBatchedDropsMalformedAtDecode(t *testing.T) {
	var got []model.TemperatureMessage
	handler, closeBuffer := NewBatched(
		func(msg RawMessage) (model.TemperatureMessage, error) {
			return model.ParseTemperatureMessage(msg.Payload)
		},
		func(_ context.Context, items []model.TemperatureMessage) {
			got = append(got, items...)
		},
		discardLogger(),
	)

	handler(context.Background(), RawMessage{Topic: "temperature", Payload: []byte("23.5")})
	// A malformed payload is dropped in the handler rather than decoded to a
	// zero-value reading — matching the Rainfall handler's semantics.
	handler(context.Background(), RawMessage{Topic: "temperature", Payload: []byte("not-a-number")})
	closeBuffer()

	if len(got) != 1 {
		t.Fatalf("batched = %v, want only the valid reading", got)
	}
	if got[0].Temperature != 23.5 {
		t.Errorf("reading = %v, want 23.5", got[0].Temperature)
	}
	if got[0].ReceivedAt.IsZero() {
		t.Error("reading's ReceivedAt is zero; decode must stamp it")
	}
}

// TestNewTemperatureHandlerFlushesToStore crosses the constructor's whole
// handling story — decode, buffer, flush — through its own close function:
// only the valid reading reaches the store, with its decode-time stamp.
func TestNewTemperatureHandlerFlushesToStore(t *testing.T) {
	store := &fakeStore{}
	handler, closeBuffer := NewTemperatureHandler(store, discardLogger())

	handler(context.Background(), RawMessage{Topic: "temperature", Payload: []byte("23.5")})
	handler(context.Background(), RawMessage{Topic: "temperature", Payload: []byte("not-a-number")})
	closeBuffer()

	if len(store.tempRows) != 1 {
		t.Fatalf("flushed rows = %v, want only the valid reading", store.tempRows)
	}
	if store.tempRows[0].Value != 23.5 {
		t.Errorf("row value = %v, want 23.5", store.tempRows[0].Value)
	}
	if store.tempRows[0].ReceivedAt.IsZero() {
		t.Error("row ReceivedAt is zero; decode must stamp it")
	}
}

// A failed flush is logged and dropped inside the closure (at-most-once) —
// close completes regardless; the failing batch must still have reached the
// store, proving the flush ran rather than the error being swallowed upstream.
func TestNewTemperatureHandlerDropsBatchOnStoreError(t *testing.T) {
	store := &fakeStore{tempErr: context.DeadlineExceeded}
	handler, closeBuffer := NewTemperatureHandler(store, discardLogger())

	handler(context.Background(), RawMessage{Topic: "temperature", Payload: []byte("21")})

	closeBuffer()
	if len(store.tempRows) != 1 {
		t.Fatalf("flush attempted rows = %v, want the failing batch to have reached the store", store.tempRows)
	}
}

// The humidity declaration crosses the seam the same way temperature does:
// only the valid reading reaches the store, with its decode-time stamp.
func TestNewHumidityHandlerFlushesToStore(t *testing.T) {
	store := &fakeStore{}
	handler, closeBuffer := NewHumidityHandler(store, discardLogger())

	handler(context.Background(), RawMessage{Topic: "humidity", Payload: []byte("55.5")})
	handler(context.Background(), RawMessage{Topic: "humidity", Payload: []byte("not-a-number")})
	closeBuffer()

	if len(store.humRows) != 1 {
		t.Fatalf("flushed rows = %v, want only the valid reading", store.humRows)
	}
	if store.humRows[0].Value != 55.5 {
		t.Errorf("row value = %v, want 55.5", store.humRows[0].Value)
	}
	if store.humRows[0].ReceivedAt.IsZero() {
		t.Error("row ReceivedAt is zero; decode must stamp it")
	}
}
