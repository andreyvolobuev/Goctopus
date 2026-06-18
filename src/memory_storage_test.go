package main

import (
	"sort"
	"testing"

	"github.com/google/uuid"
)

func TestMemoryStorageAddAndGet(t *testing.T) {
	s := &MemoryStorage{}
	s.Init()

	s.AddMessage("k", Message{id: uuid.New(), Key: "k", Value: 1})
	s.AddMessage("k", Message{id: uuid.New(), Key: "k", Value: 2})

	q, err := s.GetQueue("k")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q) != 2 {
		t.Fatalf("queue len = %d, want 2", len(q))
	}
}

func TestMemoryStorageDeleteQueue(t *testing.T) {
	s := &MemoryStorage{}
	s.Init()
	s.AddMessage("k", Message{id: uuid.New(), Key: "k", Value: 1})

	s.DeleteQueue("k")

	q, _ := s.GetQueue("k")
	if len(q) != 0 {
		t.Fatalf("queue should be empty after delete, got %d", len(q))
	}
}

func TestMemoryStorageGetKeys(t *testing.T) {
	s := &MemoryStorage{}
	s.Init()
	s.AddMessage("a", Message{id: uuid.New(), Key: "a", Value: 1})
	s.AddMessage("b", Message{id: uuid.New(), Key: "b", Value: 1})

	keys, _ := s.GetKeys()
	sort.Strings(keys)
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "b" {
		t.Fatalf("keys = %v, want [a b]", keys)
	}
}

func TestAuthResponseExport(t *testing.T) {
	r := AuthResponse{}
	r.User.Email = "a@b.com"
	r.User.Ogranization = "acme"
	got := r.Export()
	if len(got) != 2 {
		t.Fatalf("export = %v, want 2 keys", got)
	}

	empty := AuthResponse{}
	if len(empty.Export()) != 0 {
		t.Fatal("empty auth response should export no keys")
	}
}
