package marketdata

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiterSpacesRequests(t *testing.T) {
	// 600/min = 100ms spacing. Four calls must take at least 3 intervals.
	limiter := NewRateLimiter(600)
	ctx := context.Background()

	start := time.Now()
	for i := 0; i < 4; i++ {
		if err := limiter.Wait(ctx); err != nil {
			t.Fatalf("wait %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)

	if min := 300 * time.Millisecond; elapsed < min {
		t.Fatalf("expected at least %v for 4 spaced calls, got %v", min, elapsed)
	}
}

func TestRateLimiterUnlimited(t *testing.T) {
	// perMinute <= 0 disables limiting: calls return immediately.
	limiter := NewRateLimiter(0)
	start := time.Now()
	for i := 0; i < 100; i++ {
		if err := limiter.Wait(context.Background()); err != nil {
			t.Fatalf("wait: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("unlimited limiter should not block, took %v", elapsed)
	}
}

func TestRateLimiterRespectsContext(t *testing.T) {
	// 6/min = 10s spacing. The second call should be cut short by a cancelled ctx.
	limiter := NewRateLimiter(6)
	if err := limiter.Wait(context.Background()); err != nil {
		t.Fatalf("first wait: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := limiter.Wait(ctx); err == nil {
		t.Fatal("expected context deadline error on the second (spaced) call")
	}
}
