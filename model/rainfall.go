package model

import (
	"encoding/json"
	"fmt"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type RainfallMessage struct {
	MessageType string
	Rainfall    float64 `json:"rainfall"`
}

func ParseRainfallMessage(msg mqtt.Message) RainfallMessage {
	var rainfallMessage RainfallMessage
	err := json.Unmarshal(msg.Payload(), &rainfallMessage)
	if err != nil {
		fmt.Println(err)
	}
	return rainfallMessage
}
