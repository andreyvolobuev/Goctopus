package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
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

// wsConn reads through the buffered reader returned by ws.Dial (which may hold
// frame bytes read alongside the handshake) while writes go to the connection.
type wsConn struct {
	net.Conn
	r io.Reader
}

func (c wsConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// dialWS dials the websocket endpoint, returning a connection that won't lose
// frames buffered during the handshake.
func dialWS(t *testing.T, d ws.Dialer, url string) wsConn {
	t.Helper()
	conn, br, _, err := d.Dial(context.Background(), url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	c := wsConn{Conn: conn, r: conn}
	if br != nil {
		c.r = br
	}
	return c
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
	app := newTestAppCfg(t, withCreds)
	ts := httptest.NewServer(app)
	defer ts.Close()

	conn := dialWS(t, ws.Dialer{}, wsURL(ts))
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
	app := newTestAppCfg(t, withCreds)
	ts := httptest.NewServer(app)
	defer ts.Close()

	conn := dialWS(t, ws.Dialer{}, wsURL(ts))
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

// A connection subscribed to a wildcard pattern receives messages published to
// any matching concrete key.
func TestWebsocketWildcardDelivery(t *testing.T) {
	app := newTestAppCfg(t, func(c *Config) { c.AuthURL = "org.*"; withCreds(c) }) // pattern + creds
	ts := httptest.NewServer(app)
	defer ts.Close()

	conn := dialWS(t, ws.Dialer{}, wsURL(ts))
	defer conn.Close()
	waitFor(t, func() bool { return connCount(app, "org.*") == 1 }, "pattern connection registered")

	body := strings.NewReader(`{"key":"org.sales","value":{"hello":"world"}}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/", body)
	req.SetBasicAuth("admin", "secret")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	r.Body.Close()

	data, _, err := wsutil.ReadServerData(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "world") {
		t.Fatalf("pattern subscriber did not receive message: %s", data)
	}
}

// A wildcard subscriber connecting after messages were queued receives the
// existing backlog for matching keys.
func TestWebsocketWildcardBacklog(t *testing.T) {
	app := newTestAppCfg(t, func(c *Config) { c.AuthURL = "org.*"; withCreds(c) })
	ts := httptest.NewServer(app)
	defer ts.Close()

	// Publish before anyone is connected.
	body := strings.NewReader(`{"key":"org.support","value":{"n":7}}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/", body)
	req.SetBasicAuth("admin", "secret")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	r.Body.Close()
	waitFor(t, func() bool { return queueLen(app, "org.support") > 0 }, "message queued")

	conn := dialWS(t, ws.Dialer{}, wsURL(ts))
	defer conn.Close()

	data, _, err := wsutil.ReadServerData(conn)
	if err != nil {
		t.Fatalf("read backlog: %v", err)
	}
	if !strings.Contains(string(data), "\"n\":7") {
		t.Fatalf("pattern subscriber did not receive backlog: %s", data)
	}
}

// The handler works over TLS, i.e. clients can connect with wss://.
func TestWebsocketOverTLS(t *testing.T) {
	app := newTestAppCfg(t, withCreds)
	ts := httptest.NewTLSServer(app)
	defer ts.Close()

	wssURL := "wss" + strings.TrimPrefix(ts.URL, "https") + "/ws"
	conn := dialWS(t, ws.Dialer{TLSConfig: &tls.Config{InsecureSkipVerify: true}}, wssURL)
	defer conn.Close()
	waitFor(t, func() bool { return connCount(app, "testkey") == 1 }, "connection registered over tls")

	body := strings.NewReader(`{"key":"testkey","value":{"secure":true}}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/", body)
	req.SetBasicAuth("admin", "secret")
	r, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("post over tls: %v", err)
	}
	r.Body.Close()

	data, _, err := wsutil.ReadServerData(conn)
	if err != nil {
		t.Fatalf("read over tls: %v", err)
	}
	if !strings.Contains(string(data), "secure") {
		t.Fatalf("did not receive message over tls: %s", data)
	}
}

// The server proactively pings idle connections for keepalive.
func TestWebsocketServerSendsPing(t *testing.T) {
	app := newTestAppCfg(t, func(c *Config) { c.PingInterval = 30 * time.Millisecond })
	ts := httptest.NewServer(app)
	defer ts.Close()

	conn := dialWS(t, ws.Dialer{}, wsURL(ts))
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
