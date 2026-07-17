package server

import (
	"testing"
	"time"
)

func TestIPRateLimiterAllowsUpToLimitThenBlocks(t *testing.T) {
	l := newIPRateLimiter(3, time.Minute)
	now := time.Now()

	for i := 0; i < 3; i++ {
		if ok, _ := l.allow("1.2.3.4", now); !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	ok, retry := l.allow("1.2.3.4", now)
	if ok {
		t.Fatal("4th request should be blocked")
	}
	if retry <= 0 {
		t.Fatalf("expected a positive Retry-After, got %d", retry)
	}
}

func TestIPRateLimiterIsolatesIPsAndResets(t *testing.T) {
	l := newIPRateLimiter(1, time.Minute)
	now := time.Now()

	if ok, _ := l.allow("a", now); !ok {
		t.Fatal("first hit for a should pass")
	}
	if ok, _ := l.allow("b", now); !ok {
		t.Fatal("first hit for b should pass (separate bucket)")
	}
	if ok, _ := l.allow("a", now); ok {
		t.Fatal("second hit for a should be blocked")
	}
	// After the window elapses, a is allowed again.
	if ok, _ := l.allow("a", now.Add(2*time.Minute)); !ok {
		t.Fatal("a should be allowed after the window resets")
	}
}
