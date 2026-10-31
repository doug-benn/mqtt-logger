package model

import (
	"testing"
	"time"
)

func TestParseHumidityMessage(t *testing.T) {
	valid := []struct {
		payload string
		want    float64
	}{
		{`55.5`, 55.5},
		{`0`, 0},
		{`-1.25`, -1.25},
	}
	for _, tc := range valid {
		before := time.Now()
		got, err := ParseHumidityMessage([]byte(tc.payload))
		after := time.Now()
		if err != nil {
			t.Errorf("ParseHumidityMessage(%s) error: %v", tc.payload, err)
			continue
		}
		if got.Humidity != tc.want {
			t.Errorf("ParseHumidityMessage(%s) = %v, want %v", tc.payload, got.Humidity, tc.want)
		}
		// The payload carries no timestamp, so decode must have stamped one
		// inside the call window — an unset zero would land as epoch in the
		// database.
		if got.ReceivedAt.Before(before) || got.ReceivedAt.After(after) {
			t.Errorf("ParseHumidityMessage(%s) ReceivedAt = %v, want between %v and %v",
				tc.payload, got.ReceivedAt, before, after)
		}
	}

	// A payload that is not a number must be rejected, never zero-filled —
	// a fabricated 0 is indistinguishable from a real measurement downstream.
	invalid := []string{
		`not-a-number`,
		``,
		`55.5%`,
	}
	for _, payload := range invalid {
		if _, err := ParseHumidityMessage([]byte(payload)); err == nil {
			t.Errorf("ParseHumidityMessage(%q) = nil error, want error", payload)
		}
	}
}
