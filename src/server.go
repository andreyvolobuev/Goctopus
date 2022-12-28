package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gobwas/ws"
)

func (g *Goctopus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		g.handleHTTP(w, r)

	case "/ws", "/ws/":
		g.handleWs(w, r)
	}
}

func (g *Goctopus) handleHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		g.handleGet(w, r)

	case "POST":
		g.handlePost(w, r)

	default:
		g.handleMethodNotAllowed(w, r)

	}
}

func (g *Goctopus) handleWs(w http.ResponseWriter, r *http.Request) {
	keys, err := g.AuthorizationHandler(r)

	if err != nil {
		log.Printf("%s", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		log.Printf("%s", err)
		return
	}

	g.Schedule(func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		for _, key := range keys {
			conns := g.Conns[key]
			conns = append(conns, conn)
			g.Conns[key] = conns
			fmt.Printf("Conns now is: %+v\n", g.Conns)
			g.SendMessages(key)
		}
	})
}

func (g *Goctopus) handlePost(w http.ResponseWriter, r *http.Request) {
	m := Message{}
	if err := m.Unmarshal(r.Body, g.DefaultExpire); err != nil {
		log.Printf("%s\n", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	fmt.Printf("Got message: %+v\n", m)

	g.mu.Lock()
	defer g.mu.Unlock()
	g.Schedule(func() {
		q := g.Queue[m.Key]
		q = append(q, m)
		g.Queue[m.Key] = q
		fmt.Printf("QUEUE NOW IS: %+v\n", g.Queue)
		g.SendMessages(m.Key)
	})

	w.WriteHeader(http.StatusAccepted)
}

func (g *Goctopus) handleGet(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (g *Goctopus) handleMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusMethodNotAllowed)
}
