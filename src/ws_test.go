package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// wsURL turns an http test server URL into a ws:// URL pointing at /ws.
func wsURL(ts *httptest.Server) string {
	return "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
}

// connCount returns how many connections are registered for a key.
func connCount(app *Goctopus, key string) int {
	app.mu.Lock()
	defer app.mu.Unlock()
	return len(app.Conns[key])
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", msg)
}

// End-to-end: a websocket client connects, a backend POST is delivered to it,
// the client ACKs, and the message is then removed from the queue.
func TestWebsocketDeliveryAndAck(t *testing.T) {
	app := newTestApp(t)
	t.Setenv(WS_LOGIN, "admin")
	t.Setenv(WS_PASSWORD, "secret")
	ts := httptest.NewServer(app)
	defer ts.Close()

	conn, _, _, err := ws.Dial(context.Background(), wsURL(ts))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// The dummy authorizer registers every connection under "testkey".
	waitFor(t, func() bool { return connCount(app, "testkey") == 1 }, "connection registered")

	body := strings.NewReader(`{"key":"testkey","value":{"hello":"world"}}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/", body)
	req.SetBasicAuth("admin", "secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	resp.Body.Close()

	data, _, err := wsutil.ReadServerData(conn)
	if err != nil {
		t.Fatalf("read server data: %v", err)
	}
	var msg map[string]any
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal delivered message: %v", err)
	}
	id, _ := msg["id"].(string)
	if id == "" {
		t.Fatalf("delivered message has no id: %s", data)
	}

	// ACK the message; it must then be removed from the queue.
	ack, _ := json.Marshal(map[string]string{"id": id})
	if err := wsutil.WriteClientMessage(conn, ws.OpText, ack); err != nil {
		t.Fatalf("write ack: %v", err)
	}

	waitFor(t, func() bool { return queueLen(app, "testkey") == 0 }, "queue drained after ack")
}

// Without an ACK the message stays queued (at-least-once semantics) and is not
// re-pushed to the same live connection (in-flight de-duplication).
func TestWebsocketNoDuplicateWhileInFlight(t *testing.T) {
	app := newTestApp(t)
	t.Setenv(WS_LOGIN, "admin")
	t.Setenv(WS_PASSWORD, "secret")
	ts := httptest.NewServer(app)
	defer ts.Close()

	conn, _, _, err := ws.Dial(context.Background(), wsURL(ts))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	waitFor(t, func() bool { return connCount(app, "testkey") == 1 }, "connection registered")

	post := func() {
		body := strings.NewReader(`{"key":"testkey","value":1}`)
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/", body)
		req.SetBasicAuth("admin", "secret")
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		r.Body.Close()
	}

	post()
	if _, _, err := wsutil.ReadServerData(conn); err != nil {
		t.Fatalf("read first delivery: %v", err)
	}

	// A second flush for the same key (no new message, same connection) must not
	// re-deliver the already in-flight message.
	app.schedule(func() { app.sendMessages("testkey") })

	conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, _, err := wsutil.ReadServerData(conn); err == nil {
		t.Fatal("received a duplicate delivery for an in-flight message")
	}
}

// The server proactively pings idle connections for keepalive.
func TestWebsocketServerSendsPing(t *testing.T) {
	app := newTestApp(t)
	app.pingInterval = 30 * time.Millisecond
	ts := httptest.NewServer(app)
	defer ts.Close()

	conn, _, _, err := ws.Dial(context.Background(), wsURL(ts))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	gotPing := false
	for i := 0; i < 20; i++ {
		conn.SetReadDeadline(time.Now().Add(time.Second))
		h, err := ws.ReadHeader(conn)
		if err != nil {
			t.Fatalf("read header: %v", err)
		}
		if h.Length > 0 {
			io.CopyN(io.Discard, conn, h.Length)
		}
		if h.OpCode == ws.OpPing {
			gotPing = true
			break
		}
	}
	if !gotPing {
		t.Fatal("server never sent a ping frame")
	}
}
