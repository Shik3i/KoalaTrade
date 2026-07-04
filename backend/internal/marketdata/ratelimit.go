package marketdata

import (
	"context"
	"sync"
	"time"
)

// RateLimiter enforces a minimum, evenly-spaced gap between requests so a
// provider's per-minute rate limit is never exceeded — no matter how many
// callers share it (the quote poller and the history-backfill maintainer both
// hit the same providers). It is the single choke point per provider, so the
// aggregate request rate is bounded regardless of the asset mix or cadence of
// individual callers. A non-positive rate means unlimited.
type RateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

// NewRateLimiter builds a limiter that allows at most perMinute requests/minute,
// spaced evenly. perMinute <= 0 disables limiting.
func NewRateLimiter(perMinute int) *RateLimiter {
	if perMinute <= 0 {
		return &RateLimiter{}
	}
	return &RateLimiter{interval: time.Minute / time.Duration(perMinute)}
}

// Wait blocks until the next request slot is available, or returns early if ctx
// is cancelled. Concurrent callers are handed sequential slots, so the combined
// rate across all of them never exceeds the configured limit.
func (r *RateLimiter) Wait(ctx context.Context) error {
	if r == nil || r.interval <= 0 {
		return nil
	}

	r.mu.Lock()
	now := time.Now()
	if r.next.Before(now) {
		r.next = now
	}
	wait := r.next.Sub(now)
	r.next = r.next.Add(r.interval)
	r.mu.Unlock()

	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
