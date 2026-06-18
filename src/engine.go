package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
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

// sendShards is the number of per-key send mutexes. Sends for a given key are
// serialized (so the same message is never delivered twice concurrently) while
// the global mutex stays free during network I/O.
const sendShards = 256

type Goctopus struct {
	Conns map[string][]net.Conn

	mu sync.Mutex

	// sendLocks serializes delivery per key without holding the global mu
	// during network I/O. Fixed size: bounded memory, no cleanup required.
	sendLocks [sendShards]sync.Mutex

	work       chan func()
	sem        chan struct{}
	storage    Storage
	authorizer Authorizer

	metrics metrics
}

func (g *Goctopus) Start() {
	g.Log(START_APP)

	g.storage = g.getStorage()
	g.authorizer = g.getAuthorizer()

	g.Conns = make(map[string][]net.Conn)

	n_workers, err := strconv.Atoi(os.Getenv(WS_WORKERS))
	if err != nil {
		panic(WS_WORKERS_NOT_FOUND)
	}
	g.sem = make(chan struct{}, n_workers)
	g.work = make(chan func())

	go g.sweepExpired()
}

// sendLock returns the mutex that serializes delivery for a given key.
func (g *Goctopus) sendLock(key string) *sync.Mutex {
	h := fnv.New32a()
	h.Write([]byte(key))
	return &g.sendLocks[h.Sum32()%uint32(len(g.sendLocks))]
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

	// Serialize delivery per key so a message is never delivered twice
	// concurrently, without holding the global mutex during network I/O.
	sl := g.sendLock(key)
	sl.Lock()
	defer sl.Unlock()

	// Snapshot connections and the queue under the global lock, then release it
	// so the (slow, blocking) network I/O below never blocks other requests.
	g.mu.Lock()
	conns := append([]net.Conn(nil), g.Conns[key]...)
	msgQueue, err := g.getMsgQueue(key)
	g.mu.Unlock()

	if len(conns) == 0 {
		g.Log(NO_CONNS, key)
		return
	}
	if err != nil {
		g.Log(ERR_GET_MSGS, key)
		return
	}
	if len(msgQueue) == 0 {
		g.Log(NO_MSGS, key)
		return
	}

	// Deliver outside any lock. Track which messages are done (delivered or
	// expired) and which connections died so we can apply the result later.
	done := make(map[uuid.UUID]bool)
	dead := make(map[net.Conn]bool)

	for _, msg := range msgQueue {
		g.Log(TRY_SENDING_MSG, msg.id, msg.Value, key)

		if msg.isExpired() {
			g.Log(MSG_EXPIRED, msg.id)
			g.metrics.expired.Add(1)
			done[msg.id] = true
			continue
		}

		data, err := msg.marshal(false)
		if err != nil {
			g.Log(MARSHAL_ERR, err, key)
			continue
		}

		delivered := false
		for _, conn := range conns {
			if dead[conn] {
				continue
			}
			if err := g.sendMessage(conn, data, msg.id); err != nil {
				g.Log(CONN_ERR, err, key)
				conn.Close()
				dead[conn] = true
				continue
			}
			delivered = true
			g.metrics.delivered.Add(1)
		}
		if delivered {
			done[msg.id] = true
		}
	}

	// Apply results under the global lock. We rebuild the queue from its
	// *current* contents minus the done ids so that messages queued
	// concurrently during delivery are not lost.
	g.mu.Lock()
	defer g.mu.Unlock()

	current, err := g.getMsgQueue(key)
	if err == nil {
		kept := make([]Message, 0, len(current))
		for _, m := range current {
			if !done[m.id] {
				kept = append(kept, m)
			}
		}
		if len(kept) == 0 {
			g.deleteMsgQueue(key)
			g.Log(ALL_SENT, key)
		} else {
			g.updateMsgQueue(key, kept)
			g.Log(NOT_ALL_SENT, len(kept), key)
		}
	}

	if len(dead) > 0 {
		live := make([]net.Conn, 0, len(g.Conns[key]))
		for _, conn := range g.Conns[key] {
			if !dead[conn] {
				live = append(live, conn)
			}
		}
		if len(live) == 0 {
			delete(g.Conns, key)
			g.Log(ALL_CONNS_CLOSED, key)
		} else {
			g.Conns[key] = live
			g.Log(NOT_ALL_CONNS_CLOSED, len(live), key)
		}
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
	g.mu.Lock()
	defer g.mu.Unlock()

	err := g.storage.AddMessage(m.Key, m)
	if err != nil {
		g.Log(ERR_TEMPLATE, err)
		return
	}
}

func (g *Goctopus) newConn(key string, conn net.Conn) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.Conns[key] = append(g.Conns[key], conn)
	g.Log(SAVED_NEW_CONN, key)
}

func (g *Goctopus) Log(format string, v ...any) {
	verbose, err := strconv.ParseBool(os.Getenv(WS_VERBOSE))
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
	a.Init()
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
	// Filter out expired messages from the response. Their removal from storage
	// is handled by the background sweeper (see sweepExpired), so we don't
	// schedule deletion here while holding g.mu.
	maps := []map[string]any{}
	for _, m := range q {
		if !m.isExpired() {
			maps = append(maps, m.toMap(true))
		}
	}

	queue, err := json.Marshal(maps)
	if err != nil {
		m := fmt.Sprintf(ERR_TEMPLATE, err)
		g.Log(m)
		return nil, errors.New(m)
	}
	return queue, err
}

func (g *Goctopus) deleteMsgById(key string, id uuid.UUID) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.deleteMsgByIdLocked(key, id)
}

// deleteMsgByIdLocked removes a message by id. The caller MUST already hold
// g.mu. sync.Mutex is not reentrant, so calling deleteMsgById (which locks)
// while already holding g.mu would deadlock.
func (g *Goctopus) deleteMsgByIdLocked(key string, id uuid.UUID) error {
	queue, err := g.getMsgQueue(key)
	if err != nil {
		return err
	}
	for i, msg := range queue {
		if msg.id == id {
			return g.updateMsgQueue(key, append(queue[:i], queue[i+1:]...))
		}
	}
	return nil
}

// sweepExpired periodically removes expired messages from storage so that keys
// nobody ever reconnects to don't leak memory forever.
func (g *Goctopus) sweepExpired() {
	d, err := time.ParseDuration(os.Getenv(WS_SWEEP_INTERVAL))
	if err != nil || d <= 0 {
		d = time.Minute
	}
	ticker := time.NewTicker(d)
	defer ticker.Stop()

	for range ticker.C {
		g.mu.Lock()
		keys, err := g.storage.GetKeys()
		if err != nil {
			g.mu.Unlock()
			continue
		}
		for _, key := range keys {
			queue, err := g.getMsgQueue(key)
			if err != nil {
				continue
			}
			kept := make([]Message, 0, len(queue))
			removed := 0
			for _, m := range queue {
				if m.isExpired() {
					removed++
					continue
				}
				kept = append(kept, m)
			}
			if removed == 0 {
				continue
			}
			if len(kept) == 0 {
				g.deleteMsgQueue(key)
			} else {
				g.updateMsgQueue(key, kept)
			}
			g.metrics.expired.Add(uint64(removed))
			g.Log(SWEEP_EXPIRED, removed, key)
		}
		g.mu.Unlock()
	}
}
