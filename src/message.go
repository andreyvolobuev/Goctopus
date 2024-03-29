package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
)

type Message struct {
	id     uuid.UUID
	Key    string `json:"key"`
	Value  any    `json:"value"`
	Expire string `json:"expire"`
	isSent bool
	date   time.Time
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

func (m *Message) unmarshal(data io.ReadCloser) error {
	defer data.Close()

	b, err := io.ReadAll(data)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}

	if m.Key == EMPTY_STR {
		return errors.New(INVALID_KEY)
	}

	if m.Value == nil {
		return errors.New(INVALID_VALUE)
	}

	if m.Expire == EMPTY_STR {
		m.Expire = os.Getenv(WS_MSG_EXPIRE)
	}

	m.date = time.Now()
	m.id, err = uuid.NewRandom()
	if err != nil {
		return err
	}

	return nil
}

func (m *Message) isExpired() bool {
	exp, _ := time.ParseDuration(m.Expire)
	return exp < time.Since(m.date)
}
