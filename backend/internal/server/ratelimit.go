package server

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// ipRateLimiter is a small fixed-window per-IP request limiter. It bounds abuse
// of the public API (spam, scraping, brute force beyond the login lockout)
// without any external dependency. Expired windows are swept lazily so the map
// can't grow without bound.
type ipRateLimiter struct {
	mu        sync.Mutex
	windows   map[string]*rateWindow
	limit     int
	window    time.Duration
	lastSweep time.Time
}

type rateWindow struct {
	count   int
	resetAt time.Time
}

func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		windows: make(map[string]*rateWindow),
		limit:   limit,
		window:  window,
	}
}

// allow records a hit for ip and reports whether it is under the limit, plus
// the seconds until the current window resets (for Retry-After).
func (l *ipRateLimiter) allow(ip string, now time.Time) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastSweep) > l.window {
		for key, w := range l.windows {
			if now.After(w.resetAt) {
				delete(l.windows, key)
			}
		}
		l.lastSweep = now
	}

	w, ok := l.windows[ip]
	if !ok || now.After(w.resetAt) {
		l.windows[ip] = &rateWindow{count: 1, resetAt: now.Add(l.window)}
		return true, 0
	}
	if w.count >= l.limit {
		return false, int(time.Until(w.resetAt).Seconds()) + 1
	}
	w.count++
	return true, 0
}

func (l *ipRateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// r.RemoteAddr is already the client IP thanks to middleware.RealIP,
		// but may still carry a port — trim it (IPv6-safe).
		ip := r.RemoteAddr
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
		ok, retryAfter := l.allow(ip, time.Now())
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			writeJSON(w, http.StatusTooManyRequests, errorResponse{Error: "rate limit exceeded, slow down"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

