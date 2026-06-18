package main

import (
	"sync"
	"time"
)

// historyStore keeps a bounded, TTL'd ring buffer of recently published
// messages per key so a reconnecting client can fetch what it may have missed.
// It is in-process (single instance); multi-instance history would live in the
// storage backend.
type historyStore struct {
	mu    sync.Mutex
	size  int
	ttl   time.Duration
	clock func() time.Time
	byKey map[string][]histItem
}

type histItem struct {
	m  Message
	at time.Time
}

func newHistoryStore(size int, ttl time.Duration) *historyStore {
	return &historyStore{
		size:  size,
		ttl:   ttl,
		clock: time.Now,
		byKey: make(map[string][]histItem),
	}
}

func (h *historyStore) add(key string, m Message) {
	if h == nil || h.size <= 0 {
		return
	}
	now := h.clock()
	h.mu.Lock()
	defer h.mu.Unlock()

	items := append(h.prune(h.byKey[key], now), histItem{m: m, at: now})
	if len(items) > h.size {
		items = items[len(items)-h.size:]
	}
	h.byKey[key] = items
}

func (h *historyStore) get(key string) []Message {
	if h == nil || h.size <= 0 {
		return nil
	}
	now := h.clock()
	h.mu.Lock()
	defer h.mu.Unlock()

	items := h.prune(h.byKey[key], now)
	if len(items) == 0 {
		delete(h.byKey, key)
		return nil
	}
	h.byKey[key] = items
	out := make([]Message, len(items))
	for i, it := range items {
		out[i] = it.m
	}
	return out
}

// purge prunes expired items from every key and drops keys left empty, bounding
// the map for keys that received history once and then went quiet.
func (h *historyStore) purge() {
	if h == nil || h.size <= 0 || h.ttl <= 0 {
		return
	}
	now := h.clock()
	h.mu.Lock()
	defer h.mu.Unlock()
	for k, items := range h.byKey {
		kept := h.prune(items, now)
		if len(kept) == 0 {
			delete(h.byKey, k)
		} else {
			h.byKey[k] = kept
		}
	}
}

// prune drops entries older than the TTL. Caller holds the lock.
func (h *historyStore) prune(items []histItem, now time.Time) []histItem {
	if h.ttl <= 0 {
		return items
	}
	cutoff := now.Add(-h.ttl)
	i := 0
	for i < len(items) && items[i].at.Before(cutoff) {
		i++
	}
	return items[i:]
}
