package main

import (
	"strconv"
	"time"
)

// Config holds all runtime configuration, parsed once at startup from flags and
// environment variables (see main.go) and then held on the Goctopus instance.
// This replaces reading os.Getenv on hot paths (e.g. every log line) and the
// package-level config globals.
type Config struct {
	Host string
	Port string

	Workers       int
	DefaultExpire string // fallback message TTL when a message omits "expire"
	Verbose       bool
	LogJSON       bool // emit structured logs as JSON instead of text

	Login          string
	Password       string
	InsecureNoAuth bool

	StorageEngine    string
	AuthorizerEngine string

	AuthURL     string // also used as the single key by the dummy authorizer
	AuthTimeout time.Duration
	// AuthCacheTTL, when > 0, caches the proxy authorizer's result per
	// credential (cookie/authorization) for this duration so repeated connects
	// don't hit the auth backend every time. 0 disables caching.
	AuthCacheTTL time.Duration
	RedisURL     string
	// RedisKeyTTL sets a Redis EXPIRE on each queue key as a backstop so
	// abandoned keys are reclaimed even if the sweeper never runs. 0 disables.
	RedisKeyTTL time.Duration

	SweepInterval time.Duration
	PingInterval  time.Duration
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration

	// HistorySize is the per-key number of recently published messages retained
	// for the history endpoint (0 disables). HistoryTTL bounds their age.
	HistorySize int
	HistoryTTL  time.Duration

	// ReconcileInterval, when > 0, periodically re-flushes every key this
	// instance has local connections for. It is a safety net for multi-instance
	// Redis deployments where a pub/sub notification could be missed (Redis
	// pub/sub is fire-and-forget). 0 disables it (fine for single instance).
	ReconcileInterval time.Duration

	// MaxMessageBytes bounds a single POST body and a single inbound websocket
	// message, protecting against memory-exhaustion DoS.
	MaxMessageBytes int64

	// RateLimit is the per-client-IP request rate (events/sec) for the backend
	// API and websocket upgrades; 0 disables limiting. RateBurst is the bucket
	// capacity.
	RateLimit float64
	RateBurst int
	// TrustProxyHeaders makes client-IP resolution honour X-Forwarded-For /
	// X-Real-IP. Only enable behind a trusted proxy that sets them, otherwise
	// clients can spoof their IP to evade rate limiting.
	TrustProxyHeaders bool

	TLSCert string
	TLSKey  string

	// Compression enables permessage-deflate negotiation for websocket
	// connections (RFC 7692, no context takeover).
	Compression bool

	// AllowedOrigins is the whitelist of browser Origins permitted to open a
	// websocket. Empty means no restriction; "*" allows any. Requests without an
	// Origin header (non-browser clients) are always allowed.
	AllowedOrigins []string
}

// originAllowed reports whether a websocket upgrade from the given Origin header
// is permitted.
func (c *Config) originAllowed(origin string) bool {
	if origin == EMPTY_STR || len(c.AllowedOrigins) == 0 {
		return true
	}
	for _, a := range c.AllowedOrigins {
		if a == "*" || a == origin {
			return true
		}
	}
	return false
}

func (c *Config) tlsEnabled() bool { return c.TLSCert != EMPTY_STR && c.TLSKey != EMPTY_STR }

// parseDurationOr returns the parsed duration or def when s is empty/invalid.
func parseDurationOr(s string, def time.Duration) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

// parseBool parses a boolean, treating anything unparseable as false.
func parseBool(s string) bool {
	b, _ := strconv.ParseBool(s)
	return b
}
