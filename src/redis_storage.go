package main

// Redis-backed storage. Unlike the in-memory storage it is persistent (queues
// survive restarts) and shareable: combined with the Notifier pub/sub it lets
// several Goctopus instances cooperate, so a message POSTed to one instance is
// delivered to a client connected to another.
//
// Each key maps to a Redis list at "<prefix>q:<key>" whose elements are
// encoded messages. Cross-instance notifications are published on
// "<prefix>events".

import (
	"context"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	redisDefaultURL = "redis://localhost:6379/0"
	redisPrefix     = "goctopus:"
	redisQueuePfx   = redisPrefix + "q:"
	redisEvents     = redisPrefix + "events"
)

type RedisStorage struct {
	client *redis.Client
	ctx    context.Context
}

func (s *RedisStorage) queueKey(key string) string { return redisQueuePfx + key }

func (s *RedisStorage) Init(cfg *Config) error {
	url := cfg.RedisURL
	if url == EMPTY_STR {
		url = redisDefaultURL
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		return err
	}
	s.client = redis.NewClient(opt)
	s.ctx = context.Background()

	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	return s.client.Ping(ctx).Err()
}

func (s *RedisStorage) SetQueue(key string, queue []Message) error {
	qk := s.queueKey(key)
	pipe := s.client.TxPipeline()
	pipe.Del(s.ctx, qk)
	for _, m := range queue {
		b, err := m.encode()
		if err != nil {
			return err
		}
		pipe.RPush(s.ctx, qk, b)
	}
	_, err := pipe.Exec(s.ctx)
	return err
}

func (s *RedisStorage) GetQueue(key string) ([]Message, error) {
	vals, err := s.client.LRange(s.ctx, s.queueKey(key), 0, -1).Result()
	if err != nil {
		return nil, err
	}
	queue := make([]Message, 0, len(vals))
	for _, v := range vals {
		m, err := decodeMessage([]byte(v))
		if err != nil {
			return nil, err
		}
		queue = append(queue, m)
	}
	return queue, nil
}

func (s *RedisStorage) DeleteQueue(key string) error {
	return s.client.Del(s.ctx, s.queueKey(key)).Err()
}

func (s *RedisStorage) AddMessage(key string, m Message) error {
	b, err := m.encode()
	if err != nil {
		return err
	}
	return s.client.RPush(s.ctx, s.queueKey(key), b).Err()
}

func (s *RedisStorage) GetKeys() ([]string, error) {
	var keys []string
	var cursor uint64
	for {
		batch, next, err := s.client.Scan(s.ctx, cursor, redisQueuePfx+"*", 100).Result()
		if err != nil {
			return nil, err
		}
		for _, k := range batch {
			keys = append(keys, strings.TrimPrefix(k, redisQueuePfx))
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return keys, nil
}

// Notify publishes that a key has new messages so other instances flush it.
func (s *RedisStorage) Notify(key string) error {
	return s.client.Publish(s.ctx, redisEvents, key).Err()
}

// Subscribe runs handler for every key published by any instance (including
// this one). It de-duplicates downstream via the per-connection in-flight set.
func (s *RedisStorage) Subscribe(handler func(key string)) {
	pubsub := s.client.Subscribe(s.ctx, redisEvents)
	go func() {
		ch := pubsub.Channel()
		for msg := range ch {
			handler(msg.Payload)
		}
	}()
}
