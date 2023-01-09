package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
)

type Message struct {
	Key            string `json:"key"`
	Value          any    `json:"value"`
	Expire         string `json:"expire"`
	Date           time.Time
	ExpireDuration time.Duration
}

func (m *Message) Marshal() ([]byte, error) {
	data, err := json.Marshal(m.Value)
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
	exp, err := time.ParseDuration(m.Expire)

	if err != nil {
		return err
	}
	m.ExpireDuration = exp

	m.Date = time.Now()

	return nil
}

func (m *Message) IsExpired() bool {
	return m.ExpireDuration < time.Since(m.Date)
}
