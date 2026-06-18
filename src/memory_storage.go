package main

// This is a simple memory storage used in Goctopus by default.
// It stores all the messages in memory.
// Upsides: fast
// Downsides: not persistant

import "github.com/google/uuid"

type MemoryStorage struct {
	storage map[string][]Message
}

func (s *MemoryStorage) Init(cfg *Config) error {
	s.storage = make(map[string][]Message)
	return nil
}

func (s *MemoryStorage) SetQueue(key string, queue []Message) error {
	s.storage[key] = queue
	return nil
}

func (s *MemoryStorage) GetQueue(key string) ([]Message, error) {
	return s.storage[key], nil
}

func (s *MemoryStorage) DeleteQueue(key string) error {
	delete(s.storage, key)
	return nil
}

func (s *MemoryStorage) AddMessage(key string, m Message) error {
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
	keys := make([]string, len(s.storage))
	i := 0
	for k := range s.storage {
		keys[i] = k
		i++
	}
	return keys, nil
}
