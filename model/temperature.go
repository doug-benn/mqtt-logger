package model

import (
	"fmt"
	"strconv"
)

type TemperatureMessage struct {
	Temperature float64 `json:"temperature"`
}

func ParseTemperatureMessage(payload []byte) TemperatureMessage {
	var temperatureMessage TemperatureMessage

	temp, err := strconv.ParseFloat(string(payload), 64)
	if err != nil {
		fmt.Println("Error:", err)
	}

	temperatureMessage.Temperature = temp

	return temperatureMessage
}
