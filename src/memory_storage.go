package main

// This is a simple memory storage used in Goctopus by default.
// It stores all the messages in memory.
// Upsides: fast
// Downsides: not persistant
//
// MemoryStorage is internally synchronized (like the Redis backend) so the
// engine never needs to hold its global mutex around storage access.

import (
	"sync"

	"github.com/google/uuid"
)

type MemoryStorage struct {
	mu      sync.Mutex
	storage map[string][]Message
}

func (s *MemoryStorage) Init(cfg *Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.storage = make(map[string][]Message)
	return nil
}

func (s *MemoryStorage) SetQueue(key string, queue []Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.storage[key] = queue
	return nil
}

func (s *MemoryStorage) GetQueue(key string) ([]Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Return a copy so the caller can use it after the lock is released without
	// racing a concurrent AddMessage that appends to the same backing array.
	src := s.storage[key]
	if len(src) == 0 {
		return nil, nil
	}
	out := make([]Message, len(src))
	copy(out, src)
	return out, nil
}

func (s *MemoryStorage) DeleteQueue(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.storage, key)
	return nil
}

func (s *MemoryStorage) AddMessage(key string, m Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Upsert by id so re-publishing with the same (caller-supplied) id is
	// idempotent, matching the Redis HSET behaviour.
	queue := s.storage[key]
	for i := range queue {
		if queue[i].id == m.id {
			queue[i] = m
			s.storage[key] = queue
			return nil
		}
	}
	s.storage[key] = append(queue, m)
	return nil
}

func (s *MemoryStorage) DeleteMessage(key string, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	queue := s.storage[key]
	for i, m := range queue {
		if m.id == id {
			queue = append(queue[:i], queue[i+1:]...)
			if len(queue) == 0 {
				delete(s.storage, key)
			} else {
				s.storage[key] = queue
			}
			return nil
		}
	}
	return nil
}

func (s *MemoryStorage) GetKeys() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.storage))
	for k := range s.storage {
		keys = append(keys, k)
	}
	return keys, nil
}
