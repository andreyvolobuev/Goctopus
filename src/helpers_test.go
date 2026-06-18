package main

import (
	"testing"
	"time"
)

// testConfig returns a Config wired to the in-memory storage and the dummy
// authorizer (registering connections under key, which may be a wildcard
// pattern) so tests can run without external dependencies. Keepalive pings are
// pushed far out so they don't interfere with frame-reading assertions.
func testConfig(key string) *Config {
	return &Config{
		Workers:          8,
		DefaultExpire:    "30m",
		Verbose:          false,
		StorageEngine:    MEMORY,
		AuthorizerEngine: DUMMY,
		AuthURL:          key,
		SweepInterval:    time.Minute,
		PingInterval:     time.Hour,
		ReadTimeout:      30 * time.Second,
		WriteTimeout:     10 * time.Second,
		AuthTimeout:      10 * time.Second,
		MaxMessageBytes:  1 << 20,
	}
}

// withCreds configures backend POST credentials on a test config.
func withCreds(c *Config) {
	c.Login = "admin"
	c.Password = "secret"
}

// newTestApp builds a Goctopus instance with the default test config.
func newTestApp(t *testing.T) *Goctopus {
	return newTestAppCfg(t, nil)
}

// newTestAppWithKey builds a test app whose dummy authorizer registers every
// connection under the given key.
func newTestAppWithKey(t *testing.T, key string) *Goctopus {
	return newTestAppCfg(t, func(c *Config) { c.AuthURL = key })
}

// newTestAppCfg builds and starts a test app, letting the caller tweak the
// config before Start (so it is captured once and never mutated across
// goroutines).
func newTestAppCfg(t *testing.T, mutate func(*Config)) *Goctopus {
	t.Helper()
	cfg := testConfig("testkey")
	if mutate != nil {
		mutate(cfg)
	}
	app := &Goctopus{}
	app.Start(cfg)
	return app
}

// queueLen reads the queue length under the global lock so tests don't race
// with worker goroutines or the background sweeper.
func queueLen(app *Goctopus, key string) int {
	app.mu.Lock()
	defer app.mu.Unlock()
	q, _ := app.storage.GetQueue(key)
	return len(q)
}

// seed adds a message under the global lock.
func seed(app *Goctopus, m Message) {
	app.mu.Lock()
	defer app.mu.Unlock()
	app.storage.AddMessage(m.Key, m)
}
