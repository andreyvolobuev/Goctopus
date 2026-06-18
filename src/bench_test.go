package main

import (
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
)

func BenchmarkMemoryAddGet(b *testing.B) {
	s := &MemoryStorage{}
	s.Init(nil)
	m := Message{id: uuid.New(), Key: "k", Value: map[string]any{"n": 1}, Expire: "1m", date: time.Now()}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.AddMessage("k", m)
		_, _ = s.GetQueue("k")
		s.DeleteMessage("k", m.id)
	}
}

func BenchmarkMessageEncodeDecode(b *testing.B) {
	m := Message{id: uuid.New(), Key: "k", Value: map[string]any{"hello": "world"}, Expire: "30m", date: time.Now()}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc, _ := m.encode()
		_, _ = decodeMessage(enc)
	}
}

func BenchmarkClientsForKey(b *testing.B) {
	g := &Goctopus{Conns: map[string][]*client{}, patterns: map[string]bool{}}
	for i := 0; i < 1000; i++ {
		g.Conns["user."+strconv.Itoa(i)] = []*client{{}}
	}
	g.Conns["org.*"] = []*client{{}}
	g.patterns["org.*"] = true
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.clientsForKey("org.sales")
	}
}

func BenchmarkRateLimiterAllow(b *testing.B) {
	rl := newRateLimiter(1e9, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow("10.0.0.1")
	}
}
