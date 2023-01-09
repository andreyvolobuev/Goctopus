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

	AuthHandler func(*http.Request) ([]string, error)

	mu sync.Mutex

	work chan func()
	sem  chan struct{}
}

func (g *Goctopus) Start() {
	g.Log("starting Goctopus websocket app...")

	g.Queue = make(map[string][]Message)
	g.Conns = make(map[string][]net.Conn)

	if g.AuthHandler == nil {
		g.AuthHandler = g.Authorize
	}

	n_workers, err := strconv.Atoi(os.Getenv("WS_WORKERS"))
	if err != nil {
		panic("Can not parse WS_WORKERS! The value has to be an integer.")
	}
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
				g.Log("Message is expired\n")
				g.Queue[key] = g.removeMessage(queue, j)
				continue
			}

			data, err := msg.Marshal()
			if err != nil {
				g.Log("%s. Will discard this message from %s\n", err, key)
				g.Queue[key] = g.removeMessage(queue, j)
				continue
			}

			err = wsutil.WriteServerMessage(conn, ws.OpText, data)
			if err != nil {
				g.Log("%s. Will remove a conn from %s\n", err, key)
				g.Conns[key] = g.removeConn(conns, i)
				continue
			}

			g.Queue[key] = g.removeMessage(queue, j)
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

func (g *Goctopus) Log(format string, v ...any) {
	verbose, err := strconv.ParseBool(os.Getenv("WS_VERBOSE"))
	if err != nil {
		verbose = false
	}
	if verbose {
		log.Printf(format, v...)
	}
}
