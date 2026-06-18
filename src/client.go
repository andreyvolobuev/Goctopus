package main

import (
	"encoding/json"
	"io"
	"net"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/google/uuid"
)

// client wraps a single websocket connection. All frame writes go through wmu
// so that delivery, ping and pong frames never interleave on the wire. The
// inflight set lets sendMessages avoid re-pushing a message that has already
// been written to this connection and is still awaiting its ACK.
type client struct {
	conn         net.Conn
	keys         []string
	writeTimeout time.Duration

	wmu sync.Mutex // serializes frame writes

	imu      sync.Mutex
	inflight map[uuid.UUID]bool
	closed   bool
}

func newClient(conn net.Conn, keys []string, writeTimeout time.Duration) *client {
	return &client{
		conn:         conn,
		keys:         keys,
		writeTimeout: writeTimeout,
		inflight:     make(map[uuid.UUID]bool),
	}
}

// setWriteDeadline bounds a write so a stuck/slow client can't pin the per-key
// send lock or a worker indefinitely.
func (c *client) setWriteDeadline() {
	if c.writeTimeout > 0 {
		c.conn.SetWriteDeadline(time.Now().Add(c.writeTimeout))
	}
}

func (c *client) writeMessage(data []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	c.setWriteDeadline()
	return wsutil.WriteServerMessage(c.conn, ws.OpText, data)
}

func (c *client) writePing() error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	c.setWriteDeadline()
	return ws.WriteFrame(c.conn, ws.NewPingFrame(nil))
}

func (c *client) writePong(payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	c.setWriteDeadline()
	return ws.WriteFrame(c.conn, ws.NewPongFrame(payload))
}

// markInflight returns true if the caller should write the message to this
// connection. It returns false when the connection is closed or the message is
// already in flight (written and awaiting ACK).
func (c *client) markInflight(id uuid.UUID) bool {
	c.imu.Lock()
	defer c.imu.Unlock()
	if c.closed || c.inflight[id] {
		return false
	}
	c.inflight[id] = true
	return true
}

func (c *client) clearInflight(id uuid.UUID) {
	c.imu.Lock()
	defer c.imu.Unlock()
	delete(c.inflight, id)
}

func (c *client) close() {
	c.imu.Lock()
	defer c.imu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	c.conn.Close()
}

// readLoop consumes frames from the client: ACK text frames remove the
// corresponding message from the queue, pings are answered with pongs and pongs
// reset the read deadline (keepalive). It returns — unregistering the client —
// on any read error or a close frame.
func (g *Goctopus) readLoop(c *client) {
	defer g.removeClient(c)

	rd := wsutil.NewServerSideReader(c.conn)
	rd.MaxFrameSize = g.config.MaxMessageBytes // reject oversized single frames
	for {
		c.conn.SetReadDeadline(time.Now().Add(g.config.ReadTimeout))

		hdr, err := rd.NextFrame()
		if err != nil {
			return
		}
		// Bound the total message size (covers fragmented messages too).
		payload, err := io.ReadAll(io.LimitReader(rd, g.config.MaxMessageBytes+1))
		if err != nil {
			return
		}
		if int64(len(payload)) > g.config.MaxMessageBytes {
			return // oversized message, drop the connection
		}

		switch hdr.OpCode {
		case ws.OpText, ws.OpBinary:
			g.handleAck(c, payload)
		case ws.OpPing:
			if err := c.writePong(payload); err != nil {
				return
			}
		case ws.OpClose:
			return
		}
	}
}

// handleAck removes the acknowledged message from every key this connection is
// registered under.
func (g *Goctopus) handleAck(c *client, payload []byte) {
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return
	}
	idStr, ok := data["id"].(string)
	if !ok {
		return
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return
	}

	c.clearInflight(id)

	g.mu.Lock()
	for _, key := range c.keys {
		g.deleteMsgByIdLocked(key, id)
	}
	g.mu.Unlock()

	g.metrics.delivered.Add(1)
}

// pingLoop periodically pings the client. A failing write means the connection
// is gone, so it stops (readLoop's deadline will also fire and clean up).
func (g *Goctopus) pingLoop(c *client) {
	ticker := time.NewTicker(g.config.PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-g.ctx.Done():
			c.close()
			return
		case <-ticker.C:
			if err := c.writePing(); err != nil {
				c.close()
				return
			}
		}
	}
}

// removeClient closes the connection and unregisters it from all of its keys.
func (g *Goctopus) removeClient(c *client) {
	c.close()

	g.mu.Lock()
	defer g.mu.Unlock()

	for _, key := range c.keys {
		clients := g.Conns[key]
		live := make([]*client, 0, len(clients))
		for _, x := range clients {
			if x != c {
				live = append(live, x)
			}
		}
		if len(live) == 0 {
			delete(g.Conns, key)
			delete(g.patterns, key) // no-op for non-wildcard keys
		} else {
			g.Conns[key] = live
		}
	}
	g.Log(ALL_CONNS_CLOSED, c.keys)
}
