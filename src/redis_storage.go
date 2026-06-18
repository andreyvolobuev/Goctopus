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
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
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
	keyTTL time.Duration
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
	s.keyTTL = cfg.RedisKeyTTL

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
		pipe.HSet(s.ctx, qk, m.id.String(), b)
	}
	if s.keyTTL > 0 && len(queue) > 0 {
		pipe.Expire(s.ctx, qk, s.keyTTL)
	}
	_, err := pipe.Exec(s.ctx)
	return err
}

func (s *RedisStorage) GetQueue(key string) ([]Message, error) {
	vals, err := s.client.HGetAll(s.ctx, s.queueKey(key)).Result()
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
	// A hash has no order; restore FIFO order by message timestamp.
	sort.Slice(queue, func(i, j int) bool { return queue[i].date.Before(queue[j].date) })
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
	// O(1) insert into the per-key hash, field = message id.
	qk := s.queueKey(key)
	if s.keyTTL <= 0 {
		return s.client.HSet(s.ctx, qk, m.id.String(), b).Err()
	}
	pipe := s.client.TxPipeline()
	pipe.HSet(s.ctx, qk, m.id.String(), b)
	pipe.Expire(s.ctx, qk, s.keyTTL)
	_, err = pipe.Exec(s.ctx)
	return err
}

// DeleteMessage removes a single message by id in O(1) (HDEL).
func (s *RedisStorage) DeleteMessage(key string, id uuid.UUID) error {
	return s.client.HDel(s.ctx, s.queueKey(key), id.String()).Err()
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
// The goroutine stops when ctx is cancelled.
func (s *RedisStorage) Subscribe(ctx context.Context, handler func(key string)) {
	pubsub := s.client.Subscribe(ctx, redisEvents)
	go func() {
		defer pubsub.Close()
		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				handler(msg.Payload)
			}
		}
	}()
}
