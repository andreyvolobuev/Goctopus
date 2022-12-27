package main

import (
	"encoding/json"
	"log"
	"time"
)

type Message struct {
	data     map[string]any
	receiver string
	date     time.Time
}

func (m *Message) Unmarshal(data []byte) error {
	err := json.Unmarshal(data, m.data)
	if err != nil {
		log.Fatal(err)
	}
	return err
}

func (m *Message) Marshal() ([]byte, error) {
	data, err := json.Marshal(m.data)
	if err != nil {
		log.Fatal(err)
	}
	return data, err
}
