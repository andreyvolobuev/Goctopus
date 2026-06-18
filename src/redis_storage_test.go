package main

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
)

func newRedisStorage(t *testing.T) *RedisStorage {
	s, _ := newRedisStorageMR(t, &Config{})
	return s
}

func newRedisStorageMR(t *testing.T, cfg *Config) (*RedisStorage, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	cfg.RedisURL = "redis://" + mr.Addr()
	s := &RedisStorage{}
	if err := s.Init(cfg); err != nil {
		t.Fatalf("init: %v", err)
	}
	return s, mr
}

func TestRedisAddGetRoundTrip(t *testing.T) {
	s := newRedisStorage(t)

	id := uuid.New()
	now := time.Now().Truncate(time.Second).UTC()
	in := Message{id: id, Key: "alice", Value: map[string]any{"hello": "world"}, Expire: "30m", date: now}
	if err := s.AddMessage("alice", in); err != nil {
		t.Fatalf("add: %v", err)
	}

	q, err := s.GetQueue("alice")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(q) != 1 {
		t.Fatalf("queue len = %d, want 1", len(q))
	}
	got := q[0]
	if got.id != id {
		t.Errorf("id = %v, want %v", got.id, id)
	}
	if got.Expire != "30m" {
		t.Errorf("expire = %q", got.Expire)
	}
	if !got.date.Equal(now) {
		t.Errorf("date = %v, want %v", got.date, now)
	}
	if m, ok := got.Value.(map[string]any); !ok || m["hello"] != "world" {
		t.Errorf("value = %#v", got.Value)
	}
}

func TestRedisSetQueueReplaces(t *testing.T) {
	s := newRedisStorage(t)
	s.AddMessage("k", Message{id: uuid.New(), Key: "k", Value: 1, Expire: "1m", date: time.Now()})
	s.AddMessage("k", Message{id: uuid.New(), Key: "k", Value: 2, Expire: "1m", date: time.Now()})

	keep := Message{id: uuid.New(), Key: "k", Value: 3, Expire: "1m", date: time.Now()}
	if err := s.SetQueue("k", []Message{keep}); err != nil {
		t.Fatalf("setqueue: %v", err)
	}

	q, _ := s.GetQueue("k")
	if len(q) != 1 || q[0].id != keep.id {
		t.Fatalf("setqueue did not replace, got %d items", len(q))
	}
}

func TestRedisDeleteQueueAndKeys(t *testing.T) {
	s := newRedisStorage(t)
	s.AddMessage("a", Message{id: uuid.New(), Key: "a", Value: 1, Expire: "1m", date: time.Now()})
	s.AddMessage("b", Message{id: uuid.New(), Key: "b", Value: 1, Expire: "1m", date: time.Now()})

	keys, err := s.GetKeys()
	if err != nil {
		t.Fatalf("getkeys: %v", err)
	}
	sort.Strings(keys)
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "b" {
		t.Fatalf("keys = %v, want [a b]", keys)
	}

	if err := s.DeleteQueue("a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if q, _ := s.GetQueue("a"); len(q) != 0 {
		t.Fatalf("queue a not deleted")
	}
}

func TestRedisDeleteMessage(t *testing.T) {
	s := newRedisStorage(t)
	keep := uuid.New()
	drop := uuid.New()
	s.AddMessage("k", Message{id: keep, Key: "k", Value: 1, Expire: "1m", date: time.Now()})
	s.AddMessage("k", Message{id: drop, Key: "k", Value: 2, Expire: "1m", date: time.Now().Add(time.Second)})

	if err := s.DeleteMessage("k", drop); err != nil {
		t.Fatalf("delete message: %v", err)
	}
	q, _ := s.GetQueue("k")
	if len(q) != 1 || q[0].id != keep {
		t.Fatalf("expected only the kept message, got %d items", len(q))
	}
}

func TestRedisKeyTTLBackstop(t *testing.T) {
	s, mr := newRedisStorageMR(t, &Config{RedisKeyTTL: time.Minute})
	s.AddMessage("k", Message{id: uuid.New(), Key: "k", Value: 1, Expire: "1m", date: time.Now()})

	if q, _ := s.GetQueue("k"); len(q) != 1 {
		t.Fatalf("expected 1 message before TTL, got %d", len(q))
	}

	mr.FastForward(2 * time.Minute) // expire the key

	if q, _ := s.GetQueue("k"); len(q) != 0 {
		t.Fatalf("expected key to expire via TTL backstop, got %d", len(q))
	}
}

func TestRedisGetQueueSkipsCorruptEntry(t *testing.T) {
	s, mr := newRedisStorageMR(t, &Config{})

	good := Message{id: uuid.New(), Key: "k", Value: 1, Expire: "1m", date: time.Now()}
	if err := s.AddMessage("k", good); err != nil {
		t.Fatalf("add: %v", err)
	}
	// Inject an undecodable field directly into the hash.
	mr.HSet("goctopus:q:k", "corrupt", "}{not json")

	q, err := s.GetQueue("k")
	if err != nil {
		t.Fatalf("getqueue should not fail on a corrupt entry: %v", err)
	}
	if len(q) != 1 || q[0].id != good.id {
		t.Fatalf("expected only the good message, got %d", len(q))
	}
}

func TestRedisGetKeysEmpty(t *testing.T) {
	s := newRedisStorage(t)
	keys, err := s.GetKeys()
	if err != nil {
		t.Fatalf("getkeys: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected no keys, got %v", keys)
	}
}

func TestRedisNotifyPubSub(t *testing.T) {
	s := newRedisStorage(t)

	got := make(chan string, 1)
	s.Subscribe(context.Background(), func(key string) {
		select {
		case got <- key:
		default:
		}
	})
	// give the subscriber goroutine a moment to register
	time.Sleep(50 * time.Millisecond)

	if err := s.Notify("alice"); err != nil {
		t.Fatalf("notify: %v", err)
	}

	select {
	case key := <-got:
		if key != "alice" {
			t.Fatalf("got key %q, want alice", key)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive pub/sub notification")
	}
}
