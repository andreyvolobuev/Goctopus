package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type Goctopus struct {
	Queue map[string][]Message
	Conns map[string][]net.Conn

	mu sync.Mutex

	AuthorizationHandler func(*http.Request) ([]string, error)

	work chan func()
	sem  chan struct{}
}

func (g *Goctopus) Start() {
	g.Queue = make(map[string][]Message)
	g.Conns = make(map[string][]net.Conn)

	n_workers, _ := strconv.Atoi(os.Getenv("WS_WORKERS")) // there's a default value for n_workers hence no error handling here
	g.AuthorizationHandler = Authorize
	g.sem = make(chan struct{}, n_workers)
	g.work = make(chan func())
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

	for i, conn := range conns {
		for j, msg := range queue {
			if msg.IsExpired() {
				log.Printf("Message is expired\n")
				queue := g.removeMessage(queue, j)
				g.Queue[key] = queue
				continue
			}

			data, err := msg.Marshal()
			if err != nil {
				log.Printf("%s\n", err)
				queue := g.removeMessage(queue, j)
				g.Queue[key] = queue
				continue
			}

			err = wsutil.WriteServerMessage(conn, ws.OpText, data)
			if err != nil {
				log.Printf("%s\n", err)
				conns := g.removeConn(conns, i)
				g.Conns[key] = conns
				continue
			}

			queue := g.removeMessage(queue, j)
			g.Queue[key] = queue
		}
	}
}

func (g *Goctopus) QueueMessage(m Message) {
	queue := g.Queue[m.Key]
	queue = append(queue, m)
	g.Queue[m.Key] = queue
}

func (g *Goctopus) removeMessage(s []Message, i int) []Message {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

func (g *Goctopus) NewConn(key string, conn net.Conn) {
	conns := g.Conns[key]
	conns = append(conns, conn)
	g.Conns[key] = conns
}

func (g *Goctopus) removeConn(s []net.Conn, i int) []net.Conn {
	conn := s[i]
	defer conn.Close()
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}
