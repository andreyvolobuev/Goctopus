package main

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type Goctopus struct {
	Queue map[string][]Message
	Conns map[string][]net.Conn
	MsgId int

	AuthHandler func(*http.Request) ([]string, error)

	mu sync.Mutex

	work chan func()
	sem  chan struct{}
}

func (g *Goctopus) Start() {
	g.Log("starting Goctopus websocket app...")

	g.Queue = make(map[string][]Message)
	g.Conns = make(map[string][]net.Conn)

	if g.AuthHandler == nil {
		g.AuthHandler = g.Authorize
	}

	n_workers, err := strconv.Atoi(os.Getenv("WS_WORKERS"))
	if err != nil {
		panic("Can not parse WS_WORKERS! The value has to be an integer.")
	}
	g.sem = make(chan struct{}, n_workers)
	g.work = make(chan func())
}

func (g *Goctopus) Schedule(task func()) {
	select {
	case g.work <- task:
	case g.sem <- struct{}{}:
		go g.worker(task)
	}
}

func (g *Goctopus) worker(task func()) {
	defer func() { <-g.sem }()
	for {
		task()
		task = <-g.work
	}
}

func (g *Goctopus) getMsgQueue(key string) []Message {
	return g.Queue[key]
}

func (g *Goctopus) updateMsgQueue(key string, queue []Message) {
	g.Queue[key] = queue
}

func (g *Goctopus) deleteMsgQueue(key string) {
	delete(g.Queue, key)
}

func (g *Goctopus) SendMessages(key string) {
	g.Log("Start sending messages for %s\n", key)

	msgQueue := g.getMsgQueue(key)
	if len(msgQueue) == 0 {
		g.Log("No messages to send for %s. Return\n", key)
		return
	}

	if len(g.Conns[key]) == 0 {
		g.Log("No active connections for %s. Return\n", key)
		return
	}

	queue := make([]Message, 0, len(msgQueue))
	conns := make([]net.Conn, 0, len(g.Conns[key]))

	for i, msg := range msgQueue {
		g.Log("Try sending message id:%d, value: %s to %s", msg.id, msg.Value, key)

		if msg.IsExpired() {
			g.Log("Message id:%d is expired and will be discarted\n", msg.id)
			continue
		}

		data, err := msg.Marshal()
		if err != nil {
			g.Log("%s. Will discard this message from queue for %s\n", err, key)
			continue
		}

		for _, conn := range g.Conns[key] {
			err = g.SendMessage(conn, data, msg.id)
			if err != nil {
				g.Log("%s. Will remove a conn from %s\n", err, key)
				conn.Close()
				continue
			}

			if i == len(msgQueue)-1 {
				conns = append(conns, conn)
			}

			msg.isSent = true
		}

		if !msg.isSent {
			queue = append(queue, msg)
		}
	}

	if l := len(queue); l == 0 {
		g.deleteMsgQueue(key)
		g.Log("All messages for %s have been sent, the queue is empty", key)
	} else {
		g.updateMsgQueue(key, queue)
		g.Log("There are %d unsent messages tha will remain in the queue for %s", l, key)
	}

	if l := len(conns); l == 0 {
		delete(g.Conns, key)
		g.Log("All connections for %s have been closed", key)
	} else {
		g.Conns[key] = conns
		g.Log("There are %d active connections remain for %s", l, key)
	}
}

func (g *Goctopus) SendMessage(c net.Conn, d []byte, id int) error {
	if err := wsutil.WriteServerMessage(c, ws.OpText, d); err != nil {
		return err
	}

	c.SetReadDeadline(time.Now().Add(time.Second * 1))
	msg, _, err := wsutil.ReadClientData(c)
	if err != nil {
		return err
	} else {
		c.SetReadDeadline(time.Time{})
	}

	data := make(map[string]interface{})

	err = json.Unmarshal(msg, &data)
	if err != nil {
		return err
	}

	id_, ok := data["id"].(float64)
	if !ok {
		return errors.New("wrong message id type")
	}

	if id != int(id_) {
		g.Log("received confirmation for wrong id (expected for %d, got for %d)", id, int(id_))
		return errors.New("received confirmation for wrong id")
	}

	return nil
}

func (g *Goctopus) QueueMessage(m Message) {
	g.MsgId++
	m.id = g.MsgId
	g.Queue[m.Key] = append(g.Queue[m.Key], m)
}

func (g *Goctopus) NewConn(key string, conn net.Conn) {
	g.Conns[key] = append(g.Conns[key], conn)
	g.Log("Saved new connection for %s", key)
}

func (g *Goctopus) Log(format string, v ...any) {
	verbose, err := strconv.ParseBool(os.Getenv("WS_VERBOSE"))
	if err != nil {
		verbose = false
	}
	if verbose {
		log.Printf(format, v...)
	}
}
