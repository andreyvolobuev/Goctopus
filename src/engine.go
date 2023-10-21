package main

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type Goctopus struct {
	Conns map[string][]net.Conn
	MsgId int

	mu sync.Mutex

	work       chan func()
	sem        chan struct{}
	storage    Storage
	authorizer Authorizer
}

func (g *Goctopus) Start() {
	g.Log("starting Goctopus websocket app...")

	g.storage = g.getStorage()
	g.authorizer = g.getAuthorizer()

	g.Conns = make(map[string][]net.Conn)

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
	queue, err := g.storage.GetQueue(key)
	if err != nil {
		g.Log("%s", err)
	}
	return queue
}

func (g *Goctopus) updateMsgQueue(key string, queue []Message) {
	err := g.storage.SetQueue(key, queue)
	if err != nil {
		g.Log("%s", err)
	}
}

func (g *Goctopus) deleteMsgQueue(key string) {
	err := g.storage.DeleteQueue(key)
	if err != nil {
		g.Log("%s", err)
	}
}

func (g *Goctopus) SendMessages(key string) {
	g.Log("Start sending messages for %s", key)

	msgQueue := g.getMsgQueue(key)
	if len(msgQueue) == 0 {
		g.Log("No messages to send for %s. Return", key)
		return
	}

	if len(g.Conns[key]) == 0 {
		g.Log("No active connections for %s. Return", key)
		return
	}

	queue := make([]Message, 0, len(msgQueue))
	conns := make([]net.Conn, 0, len(g.Conns[key]))

	for i, msg := range msgQueue {
		g.Log("Try sending message id:%d, value: %s to %s", msg.id, msg.Value, key)

		if msg.IsExpired() {
			g.Log("Message id:%d is expired and will be discarted", msg.id)
			continue
		}

		data, err := msg.Marshal()
		if err != nil {
			g.Log("%s. Will discard this message from queue for %s", err, key)
			continue
		}

		for _, conn := range g.Conns[key] {
			err = g.SendMessage(conn, data, msg.id)
			if err != nil {
				g.Log("%s. Will remove a conn from %s", err, key)
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
	err := g.storage.AddMessage(m.Key, m)
	if err != nil {
		g.Log("%s", err)
	}
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
		log.Printf(format+"\n", v...)
	}
}

func (g *Goctopus) getStorage() Storage {
	m := Storages[strings.ToLower(storageEngine)]
	m.Init()
	g.Log("Storage %s initialized", storageEngine)
	return m
}

func (g *Goctopus) getAuthorizer() Authorizer {
	a := Authorizers[strings.ToLower(authorizerEngine)]
	g.Log("Authorizer %s will be used", storageEngine)
	return a
}
