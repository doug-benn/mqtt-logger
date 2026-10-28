package middleware

import (
	"context"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestRateLimitBurstPassesOverflowDrops(t *testing.T) {
	rec := &passRecorder{}
	// Burst 1 at one token per hour: the first message consumes the bucket
	// and nothing refills it during the test.
	limiter := rate.NewLimiter(rate.Every(time.Hour), 1)
	limit := RateLimit[int](limiter)(rec.handler())

	limit(context.Background(), 1)
	limit(context.Background(), 2)

	if len(rec.passed) != 1 || rec.passed[0] != 1 {
		t.Fatalf("passed = %v, want [1]", rec.passed)
	}
}
