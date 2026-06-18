package main

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHistoryPurgeDropsAgedKeys(t *testing.T) {
	h := newHistoryStore(10, time.Minute)
	now := time.Unix(0, 0)
	h.clock = func() time.Time { return now }

	h.add("k", Message{id: uuid.New(), Key: "k", Value: 1})
	if got := h.get("k"); len(got) != 1 {
		t.Fatalf("expected 1 history item, got %d", len(got))
	}

	now = now.Add(2 * time.Minute) // age past the TTL
	h.purge()

	h.mu.Lock()
	n := len(h.byKey)
	h.mu.Unlock()
	if n != 0 {
		t.Fatalf("expected aged-out history key to be purged, %d remain", n)
	}
}
