package model

import (
	"encoding/json"
	"fmt"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type RainfallMessage struct {
	Rainfall float64 `json:"rainfall"`
}

// ParseRainfallMessage rejects payloads that are not JSON or that carry no
// numeric rainfall field, so a bad payload can never surface as a zero reading.
func ParseRainfallMessage(msg mqtt.Message) (RainfallMessage, error) {
	var payload struct {
		Rainfall *float64 `json:"rainfall"`
	}
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		return RainfallMessage{}, fmt.Errorf("decoding rainfall payload: %w", err)
	}
	if payload.Rainfall == nil {
		return RainfallMessage{}, fmt.Errorf("payload has no rainfall field")
	}
	return RainfallMessage{Rainfall: *payload.Rainfall}, nil
}
