package main

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientIPTrustProxyHeaders(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")

	trusting := &Goctopus{config: &Config{TrustProxyHeaders: true}}
	if ip := trusting.clientIP(r); ip != "203.0.113.7" {
		t.Fatalf("trusted XFF: got %q, want 203.0.113.7", ip)
	}

	notrust := &Goctopus{config: &Config{TrustProxyHeaders: false}}
	if ip := notrust.clientIP(r); ip != "10.0.0.1" {
		t.Fatalf("untrusted: got %q, want 10.0.0.1 (RemoteAddr)", ip)
	}
}

func TestRateLimiterBurstAndRefill(t *testing.T) {
	now := time.Unix(0, 0)
	rl := newRateLimiter(10, 2) // 10/s, burst 2
	rl.clock = func() time.Time { return now }

	if !rl.Allow("ip") || !rl.Allow("ip") {
		t.Fatal("first two requests should be allowed (burst=2)")
	}
	if rl.Allow("ip") {
		t.Fatal("third immediate request should be denied")
	}

	// after 100ms at 10/s, one token is back
	now = now.Add(100 * time.Millisecond)
	if !rl.Allow("ip") {
		t.Fatal("request should be allowed after refill")
	}

	// a different IP has its own bucket
	if !rl.Allow("other") {
		t.Fatal("a different client should have its own bucket")
	}
}
