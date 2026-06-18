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

	Login          string
	Password       string
	InsecureNoAuth bool

	StorageEngine    string
	AuthorizerEngine string

	AuthURL     string // also used as the single key by the dummy authorizer
	AuthTimeout time.Duration
	RedisURL    string

	SweepInterval time.Duration
	PingInterval  time.Duration
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration

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

	TLSCert string
	TLSKey  string

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
