package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
)

type Message struct {
	id     int
	Key    string `json:"key"`
	Value  any    `json:"value"`
	Expire string `json:"expire"`
	isSent bool
	date   time.Time
}

func (m *Message) Marshal() ([]byte, error) {
	payload := make(map[string]any)
	payload["id"] = m.id
	payload["payload"] = m.Value
	data, err := json.Marshal(payload)
	return data, err
}

func (m *Message) Unmarshal(data io.ReadCloser) error {
	defer data.Close()

	b, err := io.ReadAll(data)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}

	if m.Key == "" {
		return errors.New("invalid message key")
	}

	if m.Value == nil {
		return errors.New("invalid message value")
	}

	if m.Expire == "" {
		m.Expire = os.Getenv("WS_MSG_EXPIRE")
	}

	m.date = time.Now()

	return nil
}

func (m *Message) IsExpired() bool {
	exp, _ := time.ParseDuration(m.Expire)
	return exp < time.Since(m.date)
}
