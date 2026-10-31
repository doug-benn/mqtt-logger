package model

import (
	"fmt"
	"strconv"
	"time"
)

type TemperatureMessage struct {
	Temperature float64
	// ReceivedAt is the reading's arrival of record. Temperature's payload
	// carries no timestamp, so decode stamps one; a payload that did carry
	// one would be extracted instead.
	ReceivedAt time.Time
}

// ParseTemperatureMessage decodes a bare float payload. A payload that is not
// a number is rejected so the drop decision lands in the handler, not here.
func ParseTemperatureMessage(payload []byte) (TemperatureMessage, error) {
	temp, err := strconv.ParseFloat(string(payload), 64)
	if err != nil {
		return TemperatureMessage{}, fmt.Errorf("decoding temperature payload: %w", err)
	}
	return TemperatureMessage{Temperature: temp, ReceivedAt: time.Now()}, nil
}
