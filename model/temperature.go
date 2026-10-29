package model

import (
	"fmt"
	"strconv"
)

type TemperatureMessage struct {
	Temperature float64
}

// ParseTemperatureMessage decodes a bare float payload. A payload that is not
// a number is rejected so the drop decision lands in the handler, not here.
func ParseTemperatureMessage(payload []byte) (TemperatureMessage, error) {
	temp, err := strconv.ParseFloat(string(payload), 64)
	if err != nil {
		return TemperatureMessage{}, fmt.Errorf("decoding temperature payload: %w", err)
	}
	return TemperatureMessage{Temperature: temp}, nil
}
