package model

import (
	"testing"
)

func TestParseRainfallMessage(t *testing.T) {
	valid := []struct {
		payload string
		want    float64
	}{
		{`{"rainfall": 1.5}`, 1.5},
		{`{"rainfall": 0}`, 0},
	}
	for _, tc := range valid {
		got, err := ParseRainfallMessage([]byte(tc.payload))
		if err != nil {
			t.Errorf("ParseRainfallMessage(%s) error: %v", tc.payload, err)
			continue
		}
		if got.Rainfall != tc.want {
			t.Errorf("ParseRainfallMessage(%s) = %v, want %v", tc.payload, got.Rainfall, tc.want)
		}
	}

	// Anything that could surface as a zero-value reading must be rejected.
	invalid := []string{
		`not json`,
		`{}`,
		`{"rainfall": null}`,
		`{"rainfall": "high"}`,
	}
	for _, payload := range invalid {
		if got, err := ParseRainfallMessage([]byte(payload)); err == nil {
			t.Errorf("ParseRainfallMessage(%s) = %+v, want error", payload, got)
		}
	}
}
