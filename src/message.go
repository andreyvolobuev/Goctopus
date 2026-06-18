package main

import (
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
)

type Message struct {
	id        uuid.UUID
	Key       string   `json:"key"`
	Keys      []string `json:"keys"`       // optional fan-out: deliver to several keys at once
	MessageID string   `json:"message_id"` // optional caller-supplied id for idempotent publishes
	Value     any      `json:"value"`
	Expire    string   `json:"expire"`
	isSent    bool
	date      time.Time
}

// targets returns the de-duplicated set of keys this message should be
// delivered to (the single "key" plus any "keys").
func (m *Message) targets() []string {
	seen := make(map[string]bool)
	var out []string
	add := func(k string) {
		if k != EMPTY_STR && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	add(m.Key)
	for _, k := range m.Keys {
		add(k)
	}
	return out
}

func (m *Message) toMap(extended bool) map[string]any {
	payload := make(map[string]any)
	payload["id"] = m.id
	payload["payload"] = m.Value
	if extended {
		payload["key"] = m.Key
		payload["exp"] = m.Expire
		payload["date"] = m.date
	}
	return payload
}

func (m *Message) marshal(extended bool) ([]byte, error) {
	payload := m.toMap(extended)
	data, err := json.Marshal(payload)
	return data, err
}

func (m *Message) unmarshal(data io.ReadCloser, defaultExpire string) error {
	defer data.Close()

	b, err := io.ReadAll(data)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}

	if len(m.targets()) == 0 {
		return errors.New(INVALID_KEY)
	}

	if m.Value == nil {
		return errors.New(INVALID_VALUE)
	}

	if m.Expire == EMPTY_STR {
		m.Expire = defaultExpire
	}

	if _, err := time.ParseDuration(m.Expire); err != nil {
		return errors.New(INVALID_EXPIRE)
	}

	m.date = time.Now()

	// A caller-supplied message_id makes publishes idempotent: re-posting with
	// the same id upserts instead of creating a duplicate.
	if m.MessageID != EMPTY_STR {
		id, err := uuid.Parse(m.MessageID)
		if err != nil {
			return errors.New(INVALID_MESSAGE_ID)
		}
		m.id = id
	} else {
		m.id, err = uuid.NewRandom()
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *Message) isExpired() bool {
	exp, _ := time.ParseDuration(m.Expire)
	return exp < time.Since(m.date)
}

// storedMessage is the on-the-wire representation used by persistent storage
// backends. Message's identity fields (id, date) are unexported and therefore
// invisible to encoding/json, so we round-trip through this struct.
type storedMessage struct {
	ID     uuid.UUID `json:"id"`
	Key    string    `json:"key"`
	Value  any       `json:"value"`
	Expire string    `json:"expire"`
	Date   time.Time `json:"date"`
}

// encode serializes a Message (including its id and date) for storage.
func (m *Message) encode() ([]byte, error) {
	return json.Marshal(storedMessage{
		ID:     m.id,
		Key:    m.Key,
		Value:  m.Value,
		Expire: m.Expire,
		Date:   m.date,
	})
}

// decodeMessage rebuilds a Message from its stored representation.
func decodeMessage(b []byte) (Message, error) {
	var s storedMessage
	if err := json.Unmarshal(b, &s); err != nil {
		return Message{}, err
	}
	return Message{
		id:     s.ID,
		Key:    s.Key,
		Value:  s.Value,
		Expire: s.Expire,
		date:   s.Date,
	}, nil
}
