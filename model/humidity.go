package model

import (
	"fmt"
	"strconv"
	"time"
)

type HumidityMessage struct {
	Humidity float64
	// ReceivedAt is the reading's arrival of record. Humidity's payload
	// carries no timestamp, so decode stamps one; a payload that did carry
	// one would be extracted instead.
	ReceivedAt time.Time
}

// ParseHumidityMessage decodes a bare float payload. A payload that is not
// a number is rejected so the drop decision lands in the handler, not here.
func ParseHumidityMessage(payload []byte) (HumidityMessage, error) {
	humidity, err := strconv.ParseFloat(string(payload), 64)
	if err != nil {
		return HumidityMessage{}, fmt.Errorf("decoding humidity payload: %w", err)
	}
	return HumidityMessage{Humidity: humidity, ReceivedAt: time.Now()}, nil
}
