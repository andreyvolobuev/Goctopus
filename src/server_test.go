package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// C2 regression: DELETE with an id but no key must return 400 and must NOT
// panic. The old code registered `defer mu.Unlock()` before acquiring the lock,
// so this early-return path unlocked an unlocked mutex.
func TestDeleteIdWithoutKeyReturns400(t *testing.T) {
	app := newTestAppCfg(t, func(c *Config) { c.InsecureNoAuth = true })

	req := httptest.NewRequest(http.MethodDelete, "/?id="+uuid.New().String(), nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// C1 regression: DELETE by key+id must complete and not deadlock. The old code
// held mu in handleDelete and then called deleteMsgById which tried to lock the
// same non-reentrant mutex again.
func TestDeleteByIdDoesNotDeadlock(t *testing.T) {
	app := newTestAppCfg(t, func(c *Config) { c.InsecureNoAuth = true })

	id := uuid.New()
	seed(app, Message{id: id, Key: "k", Value: "v", Expire: "30m", date: time.Now()})

	done := make(chan int, 1)
	go func() {
		req := httptest.NewRequest(http.MethodDelete, "/?key=k&id="+id.String(), nil)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		done <- w.Code
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("deadlock: DELETE by key+id hung")
	}

	if n := queueLen(app, "k"); n != 0 {
		t.Fatalf("message was not deleted, queue len = %d", n)
	}
}

// M2 regression: unknown paths must return 404 instead of an empty 200.
func TestUnknownPathReturns404(t *testing.T) {
	app := newTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want %d, got %d", http.StatusNotFound, w.Code)
	}
}

// H1 regression: when credentials are configured, an unauthenticated POST must
// be rejected.
func TestPostWithoutCredentialsRejectedWhenConfigured(t *testing.T) {
	app := newTestAppCfg(t, withCreds)

	body := strings.NewReader(`{"key":"k","value":{"a":1}}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

// H1 regression: by default (no credentials configured) POST must be rejected
// fail-closed rather than silently accepted.
func TestPostWithoutConfiguredCredentialsFailsClosed(t *testing.T) {
	app := newTestApp(t)

	body := strings.NewReader(`{"key":"k","value":{"a":1}}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want %d (fail-closed), got %d", http.StatusUnauthorized, w.Code)
	}
}

// A valid authenticated POST queues a message that can then be read back.
func TestPostThenGetMessage(t *testing.T) {
	app := newTestAppCfg(t, withCreds)

	body := strings.NewReader(`{"key":"alice","value":{"hello":"world"},"expire":"30m"}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.SetBasicAuth("admin", "secret")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("post: want %d, got %d", http.StatusAccepted, w.Code)
	}

	// allow the scheduled queue task to run
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if queueLen(app, "alice") > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/?key=alice", nil)
	getReq.SetBasicAuth("admin", "secret")
	getW := httptest.NewRecorder()
	app.ServeHTTP(getW, getReq)

	if getW.Code != http.StatusOK {
		t.Fatalf("get: want %d, got %d", http.StatusOK, getW.Code)
	}
	if !strings.Contains(getW.Body.String(), "world") {
		t.Fatalf("get body does not contain queued value: %s", getW.Body.String())
	}
}

// A POST body larger than MaxMessageBytes is rejected instead of being read
// into memory unbounded.
func TestPostBodyTooLarge(t *testing.T) {
	app := newTestAppCfg(t, func(c *Config) {
		withCreds(c)
		c.MaxMessageBytes = 32
	})

	big := strings.Repeat("x", 1024)
	body := strings.NewReader(`{"key":"k","value":"` + big + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.SetBasicAuth("admin", "secret")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want %d for oversized body, got %d", http.StatusBadRequest, w.Code)
	}
}

// Security: the backend API (GET/DELETE) must require auth, not just POST.
func TestGetAndDeleteRequireAuth(t *testing.T) {
	app := newTestAppCfg(t, withCreds)

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		req := httptest.NewRequest(method, "/", nil)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s without auth: want %d, got %d", method, http.StatusUnauthorized, w.Code)
		}
	}
}
