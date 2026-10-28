package middleware

import (
	"context"
	"testing"
	"time"
)

func TestChangedSequencePassDropPassPass(t *testing.T) {
	rec := &passRecorder{}
	gate := Changed(func(v int) int { return v })(rec.handler())

	for _, v := range []int{2, 2, 3, 2} {
		gate(context.Background(), v)
	}

	want := []int{2, 3, 2}
	assertPassed(t, rec.passed, want)
}

func TestChangedFirstMessageAlwaysPasses(t *testing.T) {
	rec := &passRecorder{}
	// An always-zero key must not trip the gate's own zero value on the
	// first message.
	gate := Changed(func(v int) int { return 0 })(rec.handler())

	gate(context.Background(), 1)
	gate(context.Background(), 2)

	if len(rec.passed) != 1 {
		t.Fatalf("passed = %v, want the first message only", rec.passed)
	}
}

func TestChangedStringKeys(t *testing.T) {
	var passed []string
	gate := Changed(func(s string) string { return s })(
		func(ctx context.Context, msg string) { passed = append(passed, msg) })

	for _, v := range []string{"on", "on", "off", "on"} {
		gate(context.Background(), v)
	}

	want := []string{"on", "off", "on"}
	assertPassed(t, passed, want)
}

func TestChangedFloatKeys(t *testing.T) {
	var passed []float64
	gate := Changed(func(v float64) float64 { return v })(
		func(ctx context.Context, msg float64) { passed = append(passed, msg) })

	// Floats compare by exact equality: a bit-different value passes, a
	// bit-identical one drops.
	for _, v := range []float64{2.5, 2.5, 2.5000001, 2.5} {
		gate(context.Background(), v)
	}

	want := []float64{2.5, 2.5000001, 2.5}
	assertPassed(t, passed, want)
}

func TestChangedTimeKeys(t *testing.T) {
	base := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	later := base.Add(time.Second)

	var passed []time.Time
	gate := Changed(func(t time.Time) time.Time { return t })(
		func(ctx context.Context, msg time.Time) { passed = append(passed, msg) })

	for _, v := range []time.Time{base, base, later, base} {
		gate(context.Background(), v)
	}

	want := []time.Time{base, later, base}
	assertPassed(t, passed, want)
}

func TestChangedKeyIgnoresPayloadTimestamp(t *testing.T) {
	// The gate keys on the decoded value: a payload timestamp moving must
	// not defeat it, and a new value must pass even at a repeated
	// timestamp.
	type reading struct {
		rainfall  float64
		timestamp time.Time
	}
	base := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

	var passed []reading
	gate := Changed(func(r reading) float64 { return r.rainfall })(
		func(ctx context.Context, msg reading) { passed = append(passed, msg) })

	gate(context.Background(), reading{rainfall: 2.5, timestamp: base})
	gate(context.Background(), reading{rainfall: 2.5, timestamp: base.Add(time.Minute)})
	gate(context.Background(), reading{rainfall: 3.0, timestamp: base.Add(time.Minute)})

	if len(passed) != 2 {
		t.Fatalf("passed = %v, want two readings", passed)
	}
	if passed[1].rainfall != 3.0 {
		t.Fatalf("second pass = %v, want the new rainfall value", passed[1])
	}
}

func assertPassed[T comparable](t *testing.T, got, want []T) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("passed = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("passed = %v, want %v", got, want)
		}
	}
}
