package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gobwas/ws"
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

// Concurrent publishes, reads and deletes must be race-free and not deadlock
// (storage is self-synchronized; g.mu only guards the connection registry).
func TestConcurrentAPINoRaceOrDeadlock(t *testing.T) {
	app := insecureApp(t)

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("k%d", i%5)
			body := strings.NewReader(`{"key":"` + key + `","value":` + fmt.Sprint(i) + `}`)
			post := httptest.NewRequest(http.MethodPost, "/", body)
			app.ServeHTTP(httptest.NewRecorder(), post)

			app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/?key="+key, nil))
			app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/?presence", nil))
		}(i)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent API access deadlocked")
	}
}

// Disconnecting clients must not leak goroutines: both readLoop and pingLoop
// have to exit promptly (pingLoop via the per-connection done channel, not only
// on the next ping tick).
func TestNoGoroutineLeakOnDisconnect(t *testing.T) {
	app := newTestApp(t)
	ts := httptest.NewServer(app)
	defer ts.Close()

	// Warm up, then take a baseline.
	for i := 0; i < 3; i++ {
		c := dialWS(t, ws.Dialer{}, wsURL(ts))
		waitFor(t, func() bool { return connCount(app, "testkey") == 1 }, "registered")
		c.Close()
		waitFor(t, func() bool { return connCount(app, "testkey") == 0 }, "unregistered")
	}
	time.Sleep(100 * time.Millisecond)
	base := runtime.NumGoroutine()

	for i := 0; i < 30; i++ {
		c := dialWS(t, ws.Dialer{}, wsURL(ts))
		waitFor(t, func() bool { return connCount(app, "testkey") == 1 }, "registered")
		c.Close()
		waitFor(t, func() bool { return connCount(app, "testkey") == 0 }, "unregistered")
	}

	// Goroutines should settle back near the baseline (pingLoops exited).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= base+5 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("goroutines did not settle: base=%d now=%d (pingLoop leak?)", base, runtime.NumGoroutine())
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
