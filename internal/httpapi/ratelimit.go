package httpapi

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// rateLimiter is a fixed-window counter per key. It is in-memory and
// per-process, which is sufficient for the single-instance v1 deployment.
type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	now     func() time.Time
	hits    map[string]*rlWindow
	maxKeys int
}

type rlWindow struct {
	start time.Time
	count int
}

func newRateLimiter(limit int, window time.Duration, now func() time.Time) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, now: now, hits: map[string]*rlWindow{}, maxKeys: 10000}
}

// Allow records a hit for key and reports whether it is within the limit.
func (l *rateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := l.now()
	w, ok := l.hits[key]
	if !ok || t.Sub(w.start) >= l.window {
		if !ok && len(l.hits) >= l.maxKeys {
			l.sweep(t)
		}
		l.hits[key] = &rlWindow{start: t, count: 1}
		return true
	}
	if w.count >= l.limit {
		return false
	}
	w.count++
	return true
}

func (l *rateLimiter) sweep(t time.Time) {
	for k, w := range l.hits {
		if t.Sub(w.start) >= l.window {
			delete(l.hits, k)
		}
	}
}

// clientIP returns the host part of RemoteAddr.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
