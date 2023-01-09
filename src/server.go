package main

import (
	"net/http"
	"os"

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
	keys, err := g.AuthHandler(r)

	if err != nil {
		g.Log("%s\n", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		g.Log("%s\n", err)
		return
	}

	g.Log("New connection for: %s\n", keys)

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
	if os.Getenv("WS_LOGIN") != "" && os.Getenv("WS_PASSWORD") != "" {
		username, password, ok := r.BasicAuth()
		if !ok {
			g.Log("Credentials for POST-request not provided!\n")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if username != os.Getenv("WS_LOGIN") || password != os.Getenv("WS_PASSWORD") {
			g.Log("POST-request with bad credentials\n")
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}

	m := Message{}
	if err := m.Unmarshal(r.Body); err != nil {
		g.Log("%s\n", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	g.Log("New message for %s (expires in: %s)\n", m.Key, m.Expire)

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
