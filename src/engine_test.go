package main

import (
	"testing"
	"time"
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
