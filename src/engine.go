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
	Queue   map[string][]Message
	Conns   map[string][]net.Conn
    MsgId   int

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
	queue := make([]Message, 0, len(g.Queue[key]))
	conns := make([]net.Conn, 0, len(g.Conns[key]))

    if len(g.Queue[key]) == 0 {
        return
    }

    if len(g.Conns[key]) == 0 {
        return
    }

    for i, msg := range g.Queue[key] {
        if msg.IsExpired() {
            g.Log("Message is expired\n")
            continue
        }

        data, err := msg.Marshal()
        if err != nil {
            g.Log("%s. Will discard this message from %s\n", err, key)
            continue
        }

        for _, conn := range g.Conns[key] {
            err = g.SendMessage(&conn, data)
			if err != nil {
				g.Log("%s. Will remove a conn from %s\n", err, key)
                conn.Close()
				continue
			}

            if i == len(g.Queue[key])-1 {
                conns = append(conns, conn)
            }

            msg.IsSent = true
		}

        if !msg.IsSent {
            queue = append(queue, msg)
        }
	}

    if len(queue) == 0 {
        delete(g.Queue, key)
    } else {
        g.Queue[key] = queue
    }

    if len(conns) == 0 {
        delete(g.Conns, key)
    } else {
        g.Conns[key] = conns
    }
}

func (g *Goctopus) SendMessage(conn *net.Conn, data []byte) error {
    err := wsutil.WriteServerMessage(*conn, ws.OpText, data)
    return err
}

func (g *Goctopus) QueueMessage(m Message) {
    g.MsgId++
    m.Id = g.MsgId
	g.Queue[m.Key] = append(g.Queue[m.Key], m)
}

func (g *Goctopus) NewConn(key string, conn net.Conn) {
	g.Conns[key] = append(g.Conns[key], conn)
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
