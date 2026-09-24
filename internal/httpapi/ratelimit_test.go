package httpapi

import (
	"net/http/httptest"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func TestRateLimiterAllowsUpToLimitPerWindow(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)}
	l := newRateLimiter(20, time.Minute, clock.now)
	for i := 0; i < 20; i++ {
		if !l.Allow("198.51.100.7") {
			t.Fatalf("request %d rejected, want allowed", i+1)
		}
	}
	if l.Allow("198.51.100.7") {
		t.Fatal("21st request allowed, want rejected")
	}
	if !l.Allow("203.0.113.9") {
		t.Fatal("other client rejected, want allowed")
	}
	clock.t = clock.t.Add(59 * time.Second)
	if l.Allow("198.51.100.7") {
		t.Fatal("request inside the window allowed, want rejected")
	}
	clock.t = clock.t.Add(time.Second)
	if !l.Allow("198.51.100.7") {
		t.Fatal("request after the window rejected, want allowed")
	}
}

func TestRateLimiterSweepsExpiredEntries(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)}
	l := newRateLimiter(1, time.Minute, clock.now)
	l.maxKeys = 3
	for _, k := range []string{"a", "b", "c"} {
		l.Allow(k)
	}
	clock.t = clock.t.Add(2 * time.Minute)
	l.Allow("d")
	if len(l.hits) != 1 {
		t.Fatalf("want expired keys swept, have %d keys", len(l.hits))
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	r.RemoteAddr = "192.0.2.10:54321"
	if got := clientIP(r); got != "192.0.2.10" {
		t.Fatalf("clientIP = %q", got)
	}
	r.RemoteAddr = "[2001:db8::1]:443"
	if got := clientIP(r); got != "2001:db8::1" {
		t.Fatalf("clientIP v6 = %q", got)
	}
	r.RemoteAddr = "unix-socket"
	if got := clientIP(r); got != "unix-socket" {
		t.Fatalf("clientIP fallback = %q", got)
	}
}
