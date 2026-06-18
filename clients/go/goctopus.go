// Package goctopus is a small client for the Goctopus websocket gateway.
//
// It ACKs every message by id, de-duplicates re-deliveries (delivery is
// at-least-once) and reconnects with backoff.
//
//	c := goctopus.New("ws://localhost:7890", func(payload any, id string) {
//	    log.Printf("got %v", payload)
//	})
//	c.Run(context.Background())
package goctopus

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// readConn overrides Read to consume from the buffered reader returned by
// ws.Dial (which may already hold frame bytes read alongside the handshake),
// while writes and the rest go to the underlying connection.
type readConn struct {
	net.Conn
	r io.Reader
}

func (c readConn) Read(p []byte) (int, error) { return c.r.Read(p) }

type Message struct {
	ID      string `json:"id"`
	Payload any    `json:"payload"`
}

type Client struct {
	URL         string
	OnMessage   func(payload any, id string)
	MinBackoff  time.Duration
	MaxBackoff  time.Duration
	DedupeLimit int

	seen      map[string]struct{}
	seenOrder []string
}

func New(url string, onMessage func(payload any, id string)) *Client {
	return &Client{
		URL:         url,
		OnMessage:   onMessage,
		MinBackoff:  500 * time.Millisecond,
		MaxBackoff:  10 * time.Second,
		DedupeLimit: 1000,
		seen:        make(map[string]struct{}),
	}
}

// Run connects and keeps reconnecting until ctx is cancelled.
func (c *Client) Run(ctx context.Context) error {
	backoff := c.MinBackoff
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.session(ctx); err == nil {
			backoff = c.MinBackoff
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > c.MaxBackoff {
			backoff = c.MaxBackoff
		}
	}
}

func (c *Client) session(ctx context.Context) error {
	conn, br, _, err := ws.Dial(ctx, c.URL)
	if err != nil {
		return err
	}
	defer conn.Close()

	// ws.Dial may have buffered the first frame(s) alongside the handshake
	// response; read through br so those bytes aren't lost.
	rc := readConn{Conn: conn, r: conn}
	if br != nil {
		rc.r = br
	}

	for {
		data, _, err := wsutil.ReadServerData(rc)
		if err != nil {
			return err
		}
		var m Message
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		// ACK first so the server stops re-delivering.
		ack, _ := json.Marshal(map[string]string{"id": m.ID})
		if err := wsutil.WriteClientMessage(conn, ws.OpText, ack); err != nil {
			return err
		}
		if c.markSeen(m.ID) {
			c.OnMessage(m.Payload, m.ID)
		}
	}
}

func (c *Client) markSeen(id string) bool {
	if id == "" {
		return true
	}
	if _, ok := c.seen[id]; ok {
		return false
	}
	c.seen[id] = struct{}{}
	c.seenOrder = append(c.seenOrder, id)
	if len(c.seenOrder) > c.DedupeLimit {
		oldest := c.seenOrder[0]
		c.seenOrder = c.seenOrder[1:]
		delete(c.seen, oldest)
	}
	return true
}
