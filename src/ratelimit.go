package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// rateLimiter is a simple per-key token-bucket limiter (one bucket per client
// IP). It avoids an external dependency and bounds memory by periodically
// purging idle (fully refilled) buckets.
type rateLimiter struct {
	rate  float64 // tokens added per second
	burst float64 // bucket capacity

	mu       sync.Mutex
	buckets  map[string]*bucket
	ops      int
	clock    func() time.Time
	purgeEvr int
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(rate float64, burst int) *rateLimiter {
	b := float64(burst)
	if b < 1 {
		b = 1
	}
	return &rateLimiter{
		rate:     rate,
		burst:    b,
		buckets:  make(map[string]*bucket),
		clock:    time.Now,
		purgeEvr: 1024,
	}
}

// Allow reports whether an event for key is permitted now, consuming a token.
func (rl *rateLimiter) Allow(key string) bool {
	now := rl.clock()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.ops++
	if rl.ops%rl.purgeEvr == 0 {
		rl.purge(now)
	}

	b := rl.buckets[key]
	if b == nil {
		b = &bucket{tokens: rl.burst, last: now}
		rl.buckets[key] = b
	} else {
		b.tokens += now.Sub(b.last).Seconds() * rl.rate
		if b.tokens > rl.burst {
			b.tokens = rl.burst
		}
		b.last = now
	}

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// purge drops buckets that have fully refilled (idle clients).
func (rl *rateLimiter) purge(now time.Time) {
	for k, b := range rl.buckets {
		if b.tokens+now.Sub(b.last).Seconds()*rl.rate >= rl.burst {
			delete(rl.buckets, k)
		}
	}
}

// allow applies the limiter (if configured) to a request's client IP.
func (g *Goctopus) allow(r *http.Request) bool {
	if g.limiter == nil {
		return true
	}
	return g.limiter.Allow(g.clientIP(r))
}

// clientIP resolves the client IP for rate limiting. When TrustProxyHeaders is
// enabled (only safe behind a trusted reverse proxy that sets them), it honours
// X-Forwarded-For / X-Real-IP so limiting is per real client, not per proxy.
func (g *Goctopus) clientIP(r *http.Request) string {
	if g.config.TrustProxyHeaders {
		if xff := r.Header.Get("X-Forwarded-For"); xff != EMPTY_STR {
			if i := strings.IndexByte(xff, ','); i >= 0 {
				return strings.TrimSpace(xff[:i]) // first hop = original client
			}
			return strings.TrimSpace(xff)
		}
		if xr := r.Header.Get("X-Real-IP"); xr != EMPTY_STR {
			return strings.TrimSpace(xr)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
