package model

import (
	"encoding/json"
	"fmt"
)

type RainfallMessage struct {
	Rainfall float64 `json:"rainfall"`
}

// ParseRainfallMessage rejects payloads that are not JSON or that carry no
// numeric rainfall field, so a bad payload can never surface as a zero reading.
func ParseRainfallMessage(payload []byte) (RainfallMessage, error) {
	var decoded struct {
		Rainfall *float64 `json:"rainfall"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return RainfallMessage{}, fmt.Errorf("decoding rainfall payload: %w", err)
	}
	if decoded.Rainfall == nil {
		return RainfallMessage{}, fmt.Errorf("payload has no rainfall field")
	}
	return RainfallMessage{Rainfall: *decoded.Rainfall}, nil
}
