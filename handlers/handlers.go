package handlers

import (
	"context"
	"fmt"

	"github.com/doug-benn/mqtt-logger/batch"
	"github.com/doug-benn/mqtt-logger/model"
	"github.com/doug-benn/mqtt-logger/repository"
)

// RainfallHandler is the rainfall handler: decode, then persist.
// Failures at either step are logged and dropped (at-most-once) — the broker
// cannot be told about them.
func RainfallHandler(rainfallRepo repository.RainfallRepo) Handler[RawMessage] {
	return func(ctx context.Context, msg RawMessage) {
		reading, err := model.ParseRainfallMessage(msg.Payload)
		if err != nil {
			fmt.Printf("Dropping malformed rainfall payload from %s: %v\n", msg.Topic, err)
			return
		}
		if _, err := rainfallRepo.Create(ctx, reading.Rainfall); err != nil {
			fmt.Printf("Dropping rainfall reading (db write failed): %v\n", err)
		}
	}
}

// TemperatureHandler is the temperature handler: decode, then queue
// for batching. Malformed payloads are logged and dropped, mirroring
// the Rainfall handler.
func TemperatureHandler(tempBatch *batch.BatchProcessor[model.TemperatureMessage]) Handler[RawMessage] {
	return func(ctx context.Context, msg RawMessage) {
		processedMessage, err := model.ParseTemperatureMessage(msg.Payload)
		if err != nil {
			fmt.Printf("Dropping malformed temperature payload from %s: %v\n", msg.Topic, err)
			return
		}
		fmt.Printf("Message Processed: %+v\n", processedMessage)
		tempBatch.Add(ctx, processedMessage)
	}
}
