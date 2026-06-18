package main

import (
	"context"
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
	storage    Storage
	authorizer Authorizer

	config  *Config
	limiter *rateLimiter

	ctx    context.Context
	cancel context.CancelFunc

	metrics metrics
}

func (g *Goctopus) Start(cfg *Config) {
	g.config = cfg
	g.ctx, g.cancel = context.WithCancel(context.Background())
	g.Log(START_APP)

	g.storage = g.getStorage()
	g.authorizer = g.getAuthorizer()

	g.Conns = make(map[string][]*client)
	g.patterns = make(map[string]bool)

	if cfg.Workers <= 0 {
		panic(WS_WORKERS_NOT_FOUND)
	}
	// Fixed pool of workers draining a buffered queue. Steady-state concurrency
	// is bounded by Workers; the queue absorbs bursts.
	queueCap := cfg.Workers * 64
	if queueCap < 256 {
		queueCap = 256
	}
	g.work = make(chan func(), queueCap)
	for i := 0; i < cfg.Workers; i++ {
		go g.worker()
	}

	if cfg.RateLimit > 0 {
		g.limiter = newRateLimiter(cfg.RateLimit, cfg.RateBurst)
	}

	// If the storage backend supports cross-instance notifications (Redis),
	// flush a key locally whenever any instance reports new messages for it.
	if n, ok := g.storage.(Notifier); ok {
		n.Subscribe(g.ctx, func(key string) {
			g.schedule(func() { g.sendMessages(key) })
		})
	}

	go g.sweepExpired()
	go g.reconcileLoop()
}

// reconcileLoop periodically re-flushes every key this instance serves so that
// messages missed by a (fire-and-forget) pub/sub notification are still
// delivered. In-flight de-duplication prevents re-sending pending messages, so
// this never produces duplicates for connected clients.
func (g *Goctopus) reconcileLoop() {
	if g.config.ReconcileInterval <= 0 {
		return
	}
	ticker := time.NewTicker(g.config.ReconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
		}

		g.mu.Lock()
		keySet := make(map[string]bool)
		for k := range g.Conns {
			if hasWildcard(k) {
				for _, sk := range g.storageKeysMatching(k) {
					keySet[sk] = true
				}
			} else {
				keySet[k] = true
			}
		}
		g.mu.Unlock()

		for k := range keySet {
			g.sendMessages(k)
		}
	}
}

// Stop gracefully shuts the instance down: it stops background goroutines
// (sweeper, ping loops, storage subscription) and closes all connections.
func (g *Goctopus) Stop() {
	if g.cancel != nil {
		g.cancel()
	}
	g.mu.Lock()
	for _, clients := range g.Conns {
		for _, c := range clients {
			c.close()
		}
	}
	g.mu.Unlock()
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

// schedule enqueues a task for the worker pool. It never blocks the caller: if
// the queue is full the task runs in its own goroutine (overflow valve).
func (g *Goctopus) schedule(task func()) {
	select {
	case g.work <- task:
	default:
		go g.runTask(task)
	}
}

func (g *Goctopus) worker() {
	for task := range g.work {
		g.runTask(task)
	}
}

// runTask runs a scheduled task, recovering from panics so one bad task can't
// crash the whole process (an unrecovered panic in a goroutine is fatal in Go).
func (g *Goctopus) runTask(task func()) {
	defer func() {
		if r := recover(); r != nil {
			g.Log(PANIC_RECOVERED, r)
		}
	}()
	task()
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
	if err := g.storage.DeleteMessage(key, id); err != nil {
		g.Log(ERR_TEMPLATE, err)
		return err
	}
	return nil
}

// sweepExpired periodically removes expired messages from storage so that keys
// nobody ever reconnects to don't leak memory forever.
func (g *Goctopus) sweepExpired() {
	ticker := time.NewTicker(g.config.SweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
		}

		// Snapshot the key set under a brief lock, then process each key with
		// its own short lock so the sweep never holds the global mutex for the
		// whole pass (which would stall the server under many keys).
		g.mu.Lock()
		keys, err := g.storage.GetKeys()
		g.mu.Unlock()
		if err != nil {
			continue
		}

		for _, key := range keys {
			g.sweepKey(key)
		}
	}
}

// sweepKey removes expired messages from a single key, holding the lock only
// for that key.
func (g *Goctopus) sweepKey(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	queue, err := g.getMsgQueue(key)
	if err != nil {
		return
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
		return
	}
	if len(kept) == 0 {
		g.deleteMsgQueue(key)
	} else {
		g.updateMsgQueue(key, kept)
	}
	g.metrics.expired.Add(uint64(removed))
	g.Log(SWEEP_EXPIRED, removed, key)
}
