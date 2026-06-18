package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestProxyAuthorizerCachesResult(t *testing.T) {
	var calls int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Write([]byte(`{"user":{"email":"a@b.com"}}`))
	}))
	defer backend.Close()

	a := &ProxyAuthorizer{}
	if err := a.Init(&Config{AuthURL: backend.URL, AuthTimeout: 2 * time.Second, AuthCacheTTL: time.Minute}); err != nil {
		t.Fatalf("init: %v", err)
	}
	app := &Goctopus{config: &Config{}}

	call := func() {
		req := httptest.NewRequest(http.MethodGet, "/ws", nil)
		req.Header.Set("Cookie", "session=abc")
		keys, err := a.Authorize(app, req)
		if err != nil {
			t.Fatalf("authorize: %v", err)
		}
		if len(keys) != 1 || keys[0] != "a@b.com" {
			t.Fatalf("unexpected keys: %v", keys)
		}
	}

	call()
	call()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("auth backend hit %d times, want 1 (second served from cache)", got)
	}
}

func TestProxyAuthorizerNoCacheByDefault(t *testing.T) {
	var calls int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Write([]byte(`{"user":{"email":"a@b.com"}}`))
	}))
	defer backend.Close()

	a := &ProxyAuthorizer{}
	a.Init(&Config{AuthURL: backend.URL, AuthTimeout: 2 * time.Second}) // AuthCacheTTL = 0
	app := &Goctopus{config: &Config{}}

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ws", nil)
		req.Header.Set("Cookie", "session=abc")
		if _, err := a.Authorize(app, req); err != nil {
			t.Fatalf("authorize: %v", err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("auth backend hit %d times, want 2 (caching disabled)", got)
	}
}
