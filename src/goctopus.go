package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"gopkg.in/yaml.v3"
)

type Goctopus struct {
	Queue chan Message
	Conns map[string][]net.Conn

	Hostname string `yaml:"hostname"`
	Port     string `yaml:"port"`
	QueueLen int    `yaml:"messages_queue"`
}

func (g *Goctopus) Start(filename string) {

	if filename == "" {
		filename = "goctopus.yaml"
	}

	data, err := os.ReadFile(filename)
	fmt.Println(string(data))
	if err != nil {
		log.Fatal(err)
	}

	if err := yaml.Unmarshal(data, g); err != nil {
		log.Fatal(err)
	}

	if g.QueueLen == 0 {
		g.QueueLen = 16
	}

	g.Queue = make(chan Message, g.QueueLen)
	g.Conns = make(map[string][]net.Conn)
}
