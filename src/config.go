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

	TLSCert string
	TLSKey  string
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
