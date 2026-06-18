package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// sendShards is the number of per-key send mutexes. Sends for a given key are
// serialized (so the same message is never delivered twice concurrently) while
// the global mutex stays free during network I/O.
const sendShards = 256

type Goctopus struct {
	Conns map[string][]*client

	// patterns is the set of currently-registered wildcard connection keys.
	// Tracking them separately keeps the common no-wildcard lookup O(1) while
	// still supporting pattern subscribers. Guarded by mu.
	patterns map[string]bool

	mu sync.Mutex

	// sendLocks serializes delivery per key without holding the global mu
	// during network I/O. Fixed size: bounded memory, no cleanup required.
	sendLocks [sendShards]sync.Mutex

	work       chan func()
	sem        chan struct{}
	storage    Storage
	authorizer Authorizer

	config *Config

	metrics metrics
}

func (g *Goctopus) Start(cfg *Config) {
	g.config = cfg
	g.Log(START_APP)

	g.storage = g.getStorage()
	g.authorizer = g.getAuthorizer()

	g.Conns = make(map[string][]*client)
	g.patterns = make(map[string]bool)

	if cfg.Workers <= 0 {
		panic(WS_WORKERS_NOT_FOUND)
	}
	g.sem = make(chan struct{}, cfg.Workers)
	g.work = make(chan func())

	// If the storage backend supports cross-instance notifications (Redis),
	// flush a key locally whenever any instance reports new messages for it.
	if n, ok := g.storage.(Notifier); ok {
		n.Subscribe(func(key string) {
			g.schedule(func() { g.sendMessages(key) })
		})
	}

	go g.sweepExpired()
}

// notify lets other instances know a key has new messages (no-op unless the
// storage backend is a Notifier). Called outside the global lock so the network
// publish never blocks other requests.
func (g *Goctopus) notify(key string) {
	if n, ok := g.storage.(Notifier); ok {
		if err := n.Notify(key); err != nil {
			g.Log(REDIS_NOTIFY_ERR, key, err)
		}
	}
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

// sendMessages pushes queued messages for a key to all of its connected
// clients without blocking on acknowledgement. A message is written to a
// connection at most once while in flight (see client.markInflight); it is only
// removed from the queue when the client ACKs it (handleAck) or it expires
// (sweepExpired). This makes delivery non-blocking and at-least-once.
func (g *Goctopus) sendMessages(key string) {
	g.Log(START_SENDING, key)

	// Serialize delivery per key so concurrent flushes don't race, without
	// holding the global mutex during network I/O.
	sl := g.sendLock(key)
	sl.Lock()
	defer sl.Unlock()

	g.mu.Lock()
	clients := g.clientsForKey(key)
	msgQueue, err := g.getMsgQueue(key)
	g.mu.Unlock()

	if len(clients) == 0 {
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

	for _, msg := range msgQueue {
		if msg.isExpired() {
			continue // removed from storage by the sweeper
		}

		data, err := msg.marshal(false)
		if err != nil {
			g.Log(MARSHAL_ERR, err, key)
			continue
		}

		for _, c := range clients {
			if !c.markInflight(msg.id) {
				continue // closed, or already sent and awaiting ACK
			}
			g.Log(TRY_SENDING_MSG, msg.id, msg.Value, key)
			if err := c.writeMessage(data); err != nil {
				g.Log(CONN_ERR, err, key)
				c.clearInflight(msg.id)
				c.close() // readLoop's defer unregisters it
			}
		}
	}
}

// clientsForKey returns every client that should receive a message published to
// the concrete key: those registered under the exact key plus those registered
// under a wildcard pattern that matches it. The caller must hold g.mu.
func (g *Goctopus) clientsForKey(key string) []*client {
	seen := make(map[*client]bool)
	out := make([]*client, 0, len(g.Conns[key]))

	add := func(clients []*client) {
		for _, c := range clients {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}

	add(g.Conns[key])
	for pattern := range g.patterns {
		if keyMatches(pattern, key) {
			add(g.Conns[pattern])
		}
	}
	return out
}

// storageKeysMatching returns the concrete storage keys that a wildcard
// subscription pattern covers, so a freshly-connected pattern subscriber can be
// sent the existing backlog. The caller must hold g.mu.
func (g *Goctopus) storageKeysMatching(pattern string) []string {
	keys, err := g.storage.GetKeys()
	if err != nil {
		g.Log(ERR_TEMPLATE, err)
		return nil
	}
	var out []string
	for _, k := range keys {
		if keyMatches(pattern, k) {
			out = append(out, k)
		}
	}
	return out
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

func (g *Goctopus) Log(format string, v ...any) {
	if g.config != nil && g.config.Verbose {
		log.Printf(format+"\n", v...)
	}
}

func (g *Goctopus) getStorage() Storage {
	engine := strings.ToLower(g.config.StorageEngine)
	factory, ok := Storages[engine]
	if !ok {
		panic(fmt.Sprintf(UNKNOWN_STORAGE, g.config.StorageEngine))
	}
	m := factory()
	if err := m.Init(g.config); err != nil {
		panic(fmt.Sprintf(STORAGE_INIT_ERR, g.config.StorageEngine, err))
	}
	g.Log(STORAGE_INITIALIZED, g.config.StorageEngine)
	return m
}

func (g *Goctopus) getAuthorizer() Authorizer {
	engine := strings.ToLower(g.config.AuthorizerEngine)
	factory, ok := Authorizers[engine]
	if !ok {
		panic(fmt.Sprintf(UNKNOWN_AUTHORIZER, g.config.AuthorizerEngine))
	}
	a := factory()
	if err := a.Init(g.config); err != nil {
		panic(fmt.Sprintf(AUTHORIZER_INIT_ERR, g.config.AuthorizerEngine, err))
	}
	g.Log(AUTHORIZER_INITIALIZED, g.config.AuthorizerEngine)
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
	ticker := time.NewTicker(g.config.SweepInterval)
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
