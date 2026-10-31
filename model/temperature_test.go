package model

import (
	"testing"
	"time"
)

func TestParseTemperatureMessage(t *testing.T) {
	valid := []struct {
		payload string
		want    float64
	}{
		{`23.5`, 23.5},
		{`0`, 0},
		{`-3.25`, -3.25},
	}
	for _, tc := range valid {
		before := time.Now()
		got, err := ParseTemperatureMessage([]byte(tc.payload))
		after := time.Now()
		if err != nil {
			t.Errorf("ParseTemperatureMessage(%s) error: %v", tc.payload, err)
			continue
		}
		if got.Temperature != tc.want {
			t.Errorf("ParseTemperatureMessage(%s) = %v, want %v", tc.payload, got.Temperature, tc.want)
		}
		// The payload carries no timestamp, so decode must have stamped one
		// inside the call window — an unset zero would land as epoch in the
		// database.
		if got.ReceivedAt.Before(before) || got.ReceivedAt.After(after) {
			t.Errorf("ParseTemperatureMessage(%s) ReceivedAt = %v, want between %v and %v",
				tc.payload, got.ReceivedAt, before, after)
		}
	}

	// A payload that is not a number must be rejected, never zero-filled —
	// a fabricated 0 is indistinguishable from a real measurement downstream.
	invalid := []string{
		`not-a-number`,
		``,
		`23.5C`,
	}
	for _, payload := range invalid {
		if _, err := ParseTemperatureMessage([]byte(payload)); err == nil {
			t.Errorf("ParseTemperatureMessage(%q) = nil error, want error", payload)
		}
	}
}
