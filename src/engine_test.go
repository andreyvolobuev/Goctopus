package main

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// A panicking task must not crash the process or wedge the worker pool.
func TestWorkerRecoversFromPanic(t *testing.T) {
	app := newTestApp(t)

	app.schedule(func() { panic("boom") })

	done := make(chan struct{})
	app.schedule(func() { close(done) })

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker pool stuck after a panicking task")
	}
}

// The background sweeper removes expired messages from a key nobody reconnects
// to, without holding the global lock for the whole pass.
func TestSweeperRemovesExpired(t *testing.T) {
	app := newTestAppCfg(t, func(c *Config) { c.SweepInterval = 20 * time.Millisecond })

	seed(app, Message{id: uuid.New(), Key: "k", Value: 1, Expire: "1ms", date: time.Now().Add(-time.Hour)})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if queueLen(app, "k") == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("sweeper did not remove the expired message")
}
