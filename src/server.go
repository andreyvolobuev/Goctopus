package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gobwas/ws"
	"github.com/google/uuid"
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
	case http.MethodPost:
		g.handlePost(w, r)
	case http.MethodGet:
		g.handleGet(w, r)
	case http.MethodDelete:
		g.handleDelete(w, r)

	default:
		g.handleMethodNotAllowed(w, r)

	}
}

func (g *Goctopus) handleWs(w http.ResponseWriter, r *http.Request) {
	keys, err := g.authorizer.Authorize(g, r)

	if err != nil {
		g.Log("Authentication failed: %s", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		g.Log("%s", err)
		return
	}

	g.schedule(func() {
		g.mu.Lock()
		defer g.mu.Unlock()

		for _, key := range keys {
			g.newConn(key, conn)
			g.sendMessages(key)
		}
	})
}

func (g *Goctopus) handlePost(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("WS_LOGIN") != "" && os.Getenv("WS_PASSWORD") != "" {
		username, password, ok := r.BasicAuth()
		if !ok {
			g.Log("Credentials for POST-request not provided!")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if username != os.Getenv("WS_LOGIN") || password != os.Getenv("WS_PASSWORD") {
			g.Log("POST-request with bad credentials")
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}

	m := Message{}
	if err := m.unmarshal(r.Body); err != nil {
		g.Log("%s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	g.Log("New message for %s (expires in: %s)", m.Key, m.Expire)

	g.schedule(func() {
		g.mu.Lock()
		defer g.mu.Unlock()

		g.queueMessage(m)
		g.sendMessages(m.Key)
	})

	w.WriteHeader(http.StatusAccepted)
}

func (g *Goctopus) handleGet(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	var data []byte
	if key == "" {
		g.Log("[GET] list of all available keys in storage")
		b, err := g.getMarshalledKeys()
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		data = b
	} else {
		g.Log("[GET] list of messages from key: %s", key)
		b, err := g.getMarshalledMessages(key)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		data = b
	}
	w.Write(data)
}

func (g *Goctopus) handleDelete(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	id_ := r.URL.Query().Get("id")

	// write correct and pretty log message for incomming request
	m := "message"
	if key == "" && id_ == "" {
		m = "all " + m + "s in storage"
	} else {
		if id_ == "" {
			m = fmt.Sprintf("all " + m + "s")
		} else {
			m = fmt.Sprintf(m+" with id: %s", id_)
		}
		if key != "" {
			m = fmt.Sprintf(m+" from key: %s", key)
		}
	}
	g.Log("[DELETE] " + m)

	if id_ != "" && key == "" {
		g.Log("can not handle delete request where message id %s is provided, but key is not.", id_)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if key == "" {
		keys, err := g.storage.GetKeys()
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, key := range keys {
			g.deleteMsgQueue(key)
		}
		g.Log("deleted all messages for all the keys in storage")
	} else {
		if id_ == "" {
			g.deleteMsgQueue(key)
			g.Log("deleted all messages from key: %s", key)
		} else {
			id, err := uuid.Parse(id_)
			if err != nil {
				g.Log("provided message id (%s) can not be converted to uuid", id)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			queue, err := g.getMsgQueue(key)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			for i, msg := range queue {
				if msg.id == id {
					err := g.updateMsgQueue(key, append(queue[:i], queue[i+1:]...))
					if err != nil {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
				}
			}
			g.Log("deleted message with id: %s, from key: %s", id, key)
		}
	}
}

func (g *Goctopus) handleMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusMethodNotAllowed)
}
