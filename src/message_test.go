package main

import (
	"io"
	"strings"
	"testing"
	"time"
)

func body(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }

func TestUnmarshalValidMessage(t *testing.T) {
	m := Message{}
	if err := m.unmarshal(body(`{"key":"k","value":{"a":1},"expire":"5m"}`), "30m"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Key != "k" {
		t.Fatalf("key = %q", m.Key)
	}
	if m.Expire != "5m" {
		t.Fatalf("expire = %q", m.Expire)
	}
	if m.id.String() == "00000000-0000-0000-0000-000000000000" {
		t.Fatal("id was not generated")
	}
}

func TestUnmarshalMissingKey(t *testing.T) {
	m := Message{}
	if err := m.unmarshal(body(`{"value":{"a":1}}`), "30m"); err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestUnmarshalMissingValue(t *testing.T) {
	m := Message{}
	if err := m.unmarshal(body(`{"key":"k"}`), "30m"); err == nil {
		t.Fatal("expected error for missing value")
	}
}

func TestUnmarshalDefaultsExpire(t *testing.T) {
	m := Message{}
	if err := m.unmarshal(body(`{"key":"k","value":1}`), "42m"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Expire != "42m" {
		t.Fatalf("expire = %q, want default 42m", m.Expire)
	}
}

// M3 regression: an invalid expire string must be rejected rather than silently
// making the message expire immediately.
func TestUnmarshalRejectsInvalidExpire(t *testing.T) {
	m := Message{}
	if err := m.unmarshal(body(`{"key":"k","value":1,"expire":"banana"}`), "30m"); err == nil {
		t.Fatal("expected error for invalid expire duration")
	}
}

func TestUnmarshalMultiKeyTargets(t *testing.T) {
	m := Message{}
	if err := m.unmarshal(body(`{"key":"a","keys":["b","a","c"],"value":1}`), "30m"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := m.targets()
	if len(got) != 3 {
		t.Fatalf("targets = %v, want 3 unique (a,b,c)", got)
	}
}

func TestUnmarshalKeysOnly(t *testing.T) {
	m := Message{}
	if err := m.unmarshal(body(`{"keys":["x","y"],"value":1}`), "30m"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.targets()) != 2 {
		t.Fatalf("targets = %v, want 2", m.targets())
	}
}

func TestIsExpired(t *testing.T) {
	old := Message{Expire: "10ms", date: time.Now().Add(-time.Second)}
	if !old.isExpired() {
		t.Fatal("message should be expired")
	}
	fresh := Message{Expire: "1h", date: time.Now()}
	if fresh.isExpired() {
		t.Fatal("message should not be expired")
	}
}
