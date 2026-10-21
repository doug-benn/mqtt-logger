package model

import (
	"fmt"
	"strconv"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type TemperatureMessage struct {
	Temperature float64 `json:"temperature"`
}

func ParseTemperatureMessage(msg mqtt.Message) TemperatureMessage {
	var temperatureMessage TemperatureMessage

	temp, err := strconv.ParseFloat(string(msg.Payload()), 64)
	if err != nil {
		fmt.Println("Error:", err)
	}

	temperatureMessage.Temperature = temp

	return temperatureMessage
}
