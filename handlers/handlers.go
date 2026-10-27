package handlers

import (
	"context"
	"fmt"

	"github.com/doug-benn/mqtt-logger/batch"
	"github.com/doug-benn/mqtt-logger/model"
	"github.com/doug-benn/mqtt-logger/repository"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// RainfallHandler is the rainfall handler: decode, then persist.
// Failures at either step are logged and dropped (at-most-once) — the broker
// cannot be told about them.
func RainfallHandler(rainfallRepo repository.RainfallRepo) Handler[mqtt.Message] {
	return func(ctx context.Context, msg mqtt.Message) {
		reading, err := model.ParseRainfallMessage(msg)
		if err != nil {
			fmt.Printf("Dropping malformed rainfall payload from %s: %v\n", msg.Topic(), err)
			return
		}
		if _, err := rainfallRepo.Create(ctx, reading.Rainfall); err != nil {
			fmt.Printf("Dropping rainfall reading (db write failed): %v\n", err)
		}
	}
}

// TemperatureHandler is the temperature handler: decode, then queue
// for batching. Its decode behavior is legacy and kept as-is — the parser
// logs its own errors and zero-fills.
func TemperatureHandler(tempBatch *batch.BatchProcessor[model.TemperatureMessage]) Handler[mqtt.Message] {
	return func(ctx context.Context, msg mqtt.Message) {
		processedMessage := model.ParseTemperatureMessage(msg)
		fmt.Printf("Message Processed: %+v\n", processedMessage)
		tempBatch.Add(ctx, processedMessage)
	}
}
