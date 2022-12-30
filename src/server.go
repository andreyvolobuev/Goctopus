package main

import (
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
			g.NewConn(key, conn)
			g.SendMessages(key)
		}

	})
}

func (g *Goctopus) handlePost(w http.ResponseWriter, r *http.Request) {
	m := Message{}
	if err := m.Unmarshal(r.Body); err != nil {
		log.Printf("%s\n", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	g.Schedule(func() {
		g.mu.Lock()
		defer g.mu.Unlock()

		g.QueueMessage(m)
		g.SendMessages(m.Key)
	})

	w.WriteHeader(http.StatusAccepted)
}

func (g *Goctopus) handleMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusMethodNotAllowed)
}
