package main

import (
	"context"
	"fmt"

	"github.com/doug-benn/mqtt-logger/batch"
	"github.com/doug-benn/mqtt-logger/model"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func processMQTT(message mqtt.Message, batcher *batch.BatchProcessor[model.TemperatureMessage]) {
	switch message.Topic() {
	case "temperature":
		processedMessage := model.ParseTemperatureMessage(message)
		fmt.Printf("Message Processed: %+v\n", processedMessage)
		batcher.Add(context.Background(), processedMessage)
	}
}
