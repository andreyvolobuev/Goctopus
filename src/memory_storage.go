package main

// This is a simple memory storage used in Goctopus by default.
// It stores all the messages in memory.
// Upsides: fast
// Downsides: not persistant

type MemoryStorage struct {
	storage map[string][]Message
}

func (s *MemoryStorage) Init() error {
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
	s.storage[m.Key] = append(s.storage[key], m)
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
