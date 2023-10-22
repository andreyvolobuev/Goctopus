package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/google/uuid"
)

type Goctopus struct {
	Conns map[string][]net.Conn

	mu sync.Mutex

	work       chan func()
	sem        chan struct{}
	storage    Storage
	authorizer Authorizer
}

func (g *Goctopus) Start() {
	g.Log(START_APP)

	g.storage = g.getStorage()
	g.authorizer = g.getAuthorizer()

	g.Conns = make(map[string][]net.Conn)

	n_workers, err := strconv.Atoi(os.Getenv("WS_WORKERS"))
	if err != nil {
		panic(WS_WORKERS_NOT_FOUND)
	}
	g.sem = make(chan struct{}, n_workers)
	g.work = make(chan func())
}

func (g *Goctopus) schedule(task func()) {
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

func (g *Goctopus) getMsgQueue(key string) ([]Message, error) {
	queue, err := g.storage.GetQueue(key)
	if err != nil {
		g.Log(ERR_TEMPLATE, err)
		return nil, err
	}
	return queue, nil
}

func (g *Goctopus) updateMsgQueue(key string, queue []Message) error {
	err := g.storage.SetQueue(key, queue)
	if err != nil {
		g.Log(ERR_TEMPLATE, err)
		return err
	}
	return nil
}

func (g *Goctopus) deleteMsgQueue(key string) {
	err := g.storage.DeleteQueue(key)
	if err != nil {
		g.Log(ERR_TEMPLATE, err)
	}
}

func (g *Goctopus) sendMessages(key string) {
	g.Log(START_SENDING, key)

	if len(g.Conns[key]) == 0 {
		g.Log(NO_CONNS, key)
		return
	}

	msgQueue, err := g.getMsgQueue(key)
	if err != nil {
		g.Log(ERR_GET_MSGS, key)
		return
	}

	if len(msgQueue) == 0 {
		g.Log(NO_MSGS, key)
		return
	}

	queue := make([]Message, 0, len(msgQueue))
	conns := make([]net.Conn, 0, len(g.Conns[key]))

	for i, msg := range msgQueue {
		g.Log(TRY_SENDING_MSG, msg.id, msg.Value, key)

		if msg.isExpired() {
			g.Log(MSG_EXPIRED, msg.id)
			continue
		}

		data, err := msg.marshal(false)
		if err != nil {
			g.Log(MARSHAL_ERR, err, key)
			continue
		}

		for _, conn := range g.Conns[key] {
			err = g.sendMessage(conn, data, msg.id)
			if err != nil {
				g.Log(CONN_ERR, err, key)
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
		g.Log(ALL_SENT, key)
	} else {
		g.updateMsgQueue(key, queue)
		g.Log(NOT_ALL_SENT, l, key)
	}

	if l := len(conns); l == 0 {
		delete(g.Conns, key)
		g.Log(ALL_CONNS_CLOSED, key)
	} else {
		g.Conns[key] = conns
		g.Log(NOT_ALL_CONNS_CLOSED, l, key)
	}
}

func (g *Goctopus) sendMessage(c net.Conn, d []byte, id uuid.UUID) error {
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
	id_, ok := data["id"].(string)
	if !ok {
		m := fmt.Sprintf(COULD_NOT_CONVERT_TO_ERR, BYTES)
		return errors.New(m)
	}
	received_uuid, err := uuid.Parse(id_)
	if err != nil {
		m := fmt.Sprintf(COULD_NOT_CONVERT_TO_ERR, UUID)
		return errors.New(m)
	}
	if id != received_uuid {
		m := fmt.Sprintf(WRONG_ID_CONFIRM, id, received_uuid)
		g.Log(m)
		return errors.New(m)
	}
	return nil
}

func (g *Goctopus) queueMessage(m Message) {
	err := g.storage.AddMessage(m.Key, m)
	if err != nil {
		g.Log(ERR_TEMPLATE, err)
		return
	}
}

func (g *Goctopus) newConn(key string, conn net.Conn) {
	g.Conns[key] = append(g.Conns[key], conn)
	g.Log(SAVED_NEW_CONN, key)
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
	g.Log(STORAGE_INITIALIZED, storageEngine)
	return m
}

func (g *Goctopus) getAuthorizer() Authorizer {
	a := Authorizers[strings.ToLower(authorizerEngine)]
	g.Log(AUTHORIZER_INITIALIZED, authorizerEngine)
	return a
}

func (g *Goctopus) getMarshalledKeys() ([]byte, error) {
	keys, err := g.storage.GetKeys()
	if err != nil {
		m := fmt.Sprintf(ERR_TEMPLATE, err)
		g.Log(m)
		return nil, errors.New(m)
	}
	data, err := json.Marshal(keys)
	if err != nil {
		m := fmt.Sprintf(ERR_TEMPLATE, err)
		g.Log(m)
		return nil, errors.New(m)
	}
	return data, nil
}

func (g *Goctopus) getMarshalledMessages(key string) ([]byte, error) {
	q, err := g.getMsgQueue(key)
	if err != nil {
		m := fmt.Sprintf(LIST_KEYS_ERR, key)
		g.Log(m)
		return nil, errors.New(m)
	}
	maps := []map[string]any{}
	exp := []Message{}
	for _, m := range q {
		if m.isExpired() {
			exp = append(exp, m)
		} else {
			maps = append(maps, m.toMap(true))
		}
	}

	g.schedule(func() {
		g.mu.Lock()
		defer g.mu.Unlock()

		for _, m := range exp {
			g.Log(MSG_EXPIRED, m.id)
			g.deleteMsgById(key, m.id)
		}
	})

	queue, err := json.Marshal(maps)
	if err != nil {
		m := fmt.Sprintf(ERR_TEMPLATE, err)
		g.Log(m)
		return nil, errors.New(m)
	}
	return queue, err
}

func (g *Goctopus) deleteMsgById(key string, id uuid.UUID) error {
	queue, err := g.getMsgQueue(key)
	if err != nil {
		return err
	}
	for i, msg := range queue {
		if msg.id == id {
			err := g.updateMsgQueue(key, append(queue[:i], queue[i+1:]...))
			if err != nil {
				return err
			}
		}
	}
	return nil
}
