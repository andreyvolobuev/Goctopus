package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func insecureApp(t *testing.T) *Goctopus {
	return newTestAppCfg(t, func(c *Config) { c.InsecureNoAuth = true })
}

func do(app *Goctopus, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	return w
}

func TestDeleteAllKeys(t *testing.T) {
	app := insecureApp(t)
	seed(app, Message{id: uuid.New(), Key: "a", Value: 1, Expire: "1m", date: time.Now()})
	seed(app, Message{id: uuid.New(), Key: "b", Value: 1, Expire: "1m", date: time.Now()})

	if w := do(app, http.MethodDelete, "/"); w.Code != http.StatusOK {
		t.Fatalf("delete all: got %d", w.Code)
	}
	if queueLen(app, "a") != 0 || queueLen(app, "b") != 0 {
		t.Fatal("expected all keys cleared")
	}
}

func TestDeleteAllFromKey(t *testing.T) {
	app := insecureApp(t)
	seed(app, Message{id: uuid.New(), Key: "a", Value: 1, Expire: "1m", date: time.Now()})
	seed(app, Message{id: uuid.New(), Key: "b", Value: 1, Expire: "1m", date: time.Now()})

	do(app, http.MethodDelete, "/?key=a")
	if queueLen(app, "a") != 0 {
		t.Fatal("key a should be cleared")
	}
	if queueLen(app, "b") != 1 {
		t.Fatal("key b should be untouched")
	}
}

func TestDeleteInvalidUUID(t *testing.T) {
	app := insecureApp(t)
	if w := do(app, http.MethodDelete, "/?key=a&id=not-a-uuid"); w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid uuid, got %d", w.Code)
	}
}

func TestGetKeysList(t *testing.T) {
	app := insecureApp(t)
	seed(app, Message{id: uuid.New(), Key: "alpha", Value: 1, Expire: "1m", date: time.Now()})
	seed(app, Message{id: uuid.New(), Key: "beta", Value: 1, Expire: "1m", date: time.Now()})

	w := do(app, http.MethodGet, "/")
	if w.Code != http.StatusOK {
		t.Fatalf("get keys: got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "alpha") || !strings.Contains(body, "beta") {
		t.Fatalf("keys list missing entries: %s", body)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	app := insecureApp(t)
	if w := do(app, http.MethodPut, "/"); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", w.Code)
	}
}
