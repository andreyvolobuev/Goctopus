package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"gopkg.in/yaml.v3"
)

type Goctopus struct {
	Queue   map[string][]Message
	Conns   map[string][]net.Conn
	AuthURL *url.URL

	mu sync.RWMutex

	AuthorizationHandler func(*http.Request) ([]string, error)

	work chan func()
	sem  chan struct{}

	Hostname   string `yaml:"hostname"`
	Port       string `yaml:"port"`
	WorkersLen int    `yaml:"workers"`
}

func (g *Goctopus) Start(filename string) {
	g.loadSettings(filename)
	g.launchWorkers()
}

func (g *Goctopus) loadSettings(filename string) {
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

	if g.WorkersLen == 0 {
		g.WorkersLen = 4
	}

	g.Queue = make(map[string][]Message)
	g.Conns = make(map[string][]net.Conn)
}

func (g *Goctopus) launchWorkers() {
	g.work = make(chan func())
	g.sem = make(chan struct{}, g.WorkersLen)
}

func (g *Goctopus) Schedule(task func()) {
	select {
	case g.work <- task:
	case g.sem <- struct{}{}:
		go g.worker(task)
	}
}

func (g *Goctopus) worker(task func()) {
	defer func() { <-g.sem }()
	for {
		task()
		task = <-g.work
	}
}

func (g *Goctopus) SendMessages(key string) {
	queue := g.Queue[key]
	conns := g.Conns[key]

	for _, conn := range conns {
		for i, msg := range queue {
			data, err := msg.Marshal()
			if err != nil {
				log.Printf("%s\n", err)
				continue
			}
			err = wsutil.WriteServerMessage(conn, ws.OpText, data)
			fmt.Printf("Just send %s to %s\n", data, conn)
			if err != nil {
				log.Printf("%s\n", err)
				continue
			}
			queue := g.removeMessage(queue, i)
			g.Queue[key] = queue
		}
	}
}

func (g *Goctopus) removeMessage(s []Message, i int) []Message {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}
