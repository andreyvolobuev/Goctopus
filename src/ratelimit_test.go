package main

import (
	"testing"
	"time"
)

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
