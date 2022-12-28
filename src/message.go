package main

import (
	"encoding/json"
	"errors"
	"io"
	"time"
)

type Message struct {
	Key            string    `json:"key"`
	Value          any       `json:"value"`
	Date           time.Time `json:"date"`
	Expire         string    `json:"expire"`
	ExpireDuration time.Duration
}

func (m *Message) Unmarshal(data io.ReadCloser, defaultExpire string) error {
	defer data.Close()

	b, err := io.ReadAll(data)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}

	if m.Key == "" {
		return errors.New("invalid key")
	}

	m.Date = time.Now()

	if m.Expire == "" {
		m.Expire = defaultExpire
	}
	exp, err := time.ParseDuration(m.Expire)
	if err != nil {
		return err
	}
	m.ExpireDuration = exp

	return nil
}

func (m *Message) Marshal() ([]byte, error) {
	data, err := json.Marshal(m.Value)
	return data, err
}
