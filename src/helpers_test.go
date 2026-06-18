package main

import (
	"os"
	"testing"
)

// newTestApp builds a Goctopus instance wired to the in-memory storage and the
// dummy authorizer so tests can run without an external auth backend.
func newTestApp(t *testing.T) *Goctopus {
	return newTestAppWithKey(t, "testkey")
}

// newTestAppWithKey builds a test app whose dummy authorizer registers every
// connection under the given key (which may be a wildcard pattern). The key is
// set before Start so it is captured once and never read across goroutines.
func newTestAppWithKey(t *testing.T, key string) *Goctopus {
	t.Helper()

	os.Setenv(WS_WORKERS, "8")
	os.Setenv(WS_VERBOSE, "false")
	os.Setenv(WS_MSG_EXPIRE, "30m")
	os.Setenv(WS_LOGIN, EMPTY_STR)
	os.Setenv(WS_PASSWORD, EMPTY_STR)
	// Keep keepalive pings out of the way of frame-reading assertions.
	os.Setenv(WS_PING_INTERVAL, "1h")
	os.Setenv(WS_READ_TIMEOUT, "30s")

	storageEngine = MEMORY
	authorizerEngine = DUMMY
	authUrl = key

	app := &Goctopus{}
	app.Start()
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
