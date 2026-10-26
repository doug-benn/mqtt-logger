package main

import (
	"context"
	"fmt"

	"github.com/doug-benn/mqtt-logger/batch"
	"github.com/doug-benn/mqtt-logger/model"
	"github.com/doug-benn/mqtt-logger/repository"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// processMQTT decodes one message and hands it to its sink. Failures are
// logged and dropped (at-most-once): the broker cannot be told about them.
func processMQTT(message mqtt.Message, batcher *batch.BatchProcessor[model.TemperatureMessage], rainfallRepo repository.RainfallRepo) {
	switch message.Topic() {
	case "temperature":
		processedMessage := model.ParseTemperatureMessage(message)
		fmt.Printf("Message Processed: %+v\n", processedMessage)
		batcher.Add(context.Background(), processedMessage)
	case "rainfall":
		reading, err := model.ParseRainfallMessage(message)
		if err != nil {
			fmt.Printf("Dropping malformed rainfall payload from %s: %v\n", message.Topic(), err)
			return
		}
		if _, err := rainfallRepo.Create(context.Background(), reading.Rainfall); err != nil {
			fmt.Printf("Dropping rainfall reading (db write failed): %v\n", err)
		}
	}
}
