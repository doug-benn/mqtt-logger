package handlers

import (
	"context"
	"log/slog"
	"time"

	"github.com/doug-benn/mqtt-logger/batch"
	"github.com/doug-benn/mqtt-logger/model"
	"github.com/doug-benn/mqtt-logger/repository"
)

// RainfallHandler is the rainfall handler: decode, then persist.
// Failures at either step are logged and dropped (at-most-once) — the broker
// cannot be told about them.
func RainfallHandler(repo repository.Store, logger *slog.Logger) Handler[RawMessage] {
	return func(ctx context.Context, msg RawMessage) {
		reading, err := model.ParseRainfallMessage(msg.Payload)
		if err != nil {
			logger.Warn("dropping malformed rainfall payload", "topic", msg.Topic, "err", err)
			return
		}
		if _, err := repo.CreateRainfall(ctx, reading.Rainfall); err != nil {
			logger.Error("dropping rainfall reading (db write failed)", "topic", msg.Topic, "err", err)
		}
	}
}

// The buffer values are decided policy (batch size 100, flush timeout 5s —
// the staleness contract — 2 in-flight flushes): one set for every batched
// message, deliberately not caller-supplied, so re-tuning means changing the
// decision, not a site.
const (
	batchMaxSize = 100
	flushTimeout = 5 * time.Second
	maxInFlight  = 2
)

// NewBatched is how a batched message is declared: pass its decode and its
// write, get back the chain's terminal handler plus the close function the
// wiring must call after intake drains — without it the buffered readings
// are dropped. Buffering, the decided buffer values, add/shed handling, and
// close plumbing live here once (the contract sits at
// batch.NewBatchProcessor); decode failures drop with a warn on the injected
// logger at intake, and write owns its own failure reporting — at-most-once,
// a failed flush logged and dropped like any single failed write; it is the
// closure with the context it needs.
func NewBatched[T any](decode func(RawMessage) (T, error), write func(context.Context, []T), logger *slog.Logger) (Handler[RawMessage], func()) {
	buf := batch.NewBatchProcessor(batchMaxSize, flushTimeout, maxInFlight, func(items []T) {
		write(context.Background(), items)
	})
	return func(ctx context.Context, msg RawMessage) {
		reading, err := decode(msg)
		if err != nil {
			logger.Warn("dropping malformed payload", "topic", msg.Topic, "err", err)
			return
		}
		if err := buf.Add(ctx, reading); err != nil {
			// Same at-most-once drop as a write failure: the buffer is
			// closed or shutting down and there is nowhere to retry.
			logger.Warn("dropping reading (buffer unavailable)", "topic", msg.Topic, "err", err)
		}
	}, buf.Close
}

// NewTemperatureHandler is the temperature message's declaration: its decode
// and its write, handed to the shared batching seam. The write maps the
// decode product to store rows and reports its own failures on the logger it
// is given, with the batch count.
func NewTemperatureHandler(repo repository.Store, logger *slog.Logger) (Handler[RawMessage], func()) {
	return NewBatched(
		func(msg RawMessage) (model.TemperatureMessage, error) {
			return model.ParseTemperatureMessage(msg.Payload)
		},
		func(ctx context.Context, readings []model.TemperatureMessage) {
			rows := make([]model.Temperature, len(readings))
			for i, reading := range readings {
				rows[i] = model.Temperature{Value: reading.Temperature, ReceivedAt: reading.ReceivedAt}
			}
			if err := repo.CreateTemperatureBatch(ctx, rows); err != nil {
				logger.Error("dropping temperature batch (db write failed)",
					"topic", "temperature", "count", len(readings), "err", err)
			}
		},
		logger,
	)
}

// NewHumidityHandler is the humidity message's declaration: its decode and
// its write, handed to the shared batching seam. The write maps the decode
// product to store rows and reports its own failures on the logger it is
// given, with the batch count.
func NewHumidityHandler(repo repository.Store, logger *slog.Logger) (Handler[RawMessage], func()) {
	return NewBatched(
		func(msg RawMessage) (model.HumidityMessage, error) {
			return model.ParseHumidityMessage(msg.Payload)
		},
		func(ctx context.Context, readings []model.HumidityMessage) {
			rows := make([]model.Humidity, len(readings))
			for i, reading := range readings {
				rows[i] = model.Humidity{Value: reading.Humidity, ReceivedAt: reading.ReceivedAt}
			}
			if err := repo.CreateHumidities(ctx, rows); err != nil {
				logger.Error("dropping humidity batch (db write failed)",
					"topic", "humidity", "count", len(readings), "err", err)
			}
		},
		logger,
	)
}
