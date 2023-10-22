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
	case ROOT:
		g.handleHTTP(w, r)

	case WS, WS_:
		g.handleWs(w, r)
	}
}

func (g *Goctopus) handleHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(CONTENT_TYPE, APPLICATION_JSON)

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
		g.Log(AUTH_FAILED, err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		g.Log(ERR_TEMPLATE, err)
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
	g.Log(POST_NEW_MSG)

	if os.Getenv(WS_LOGIN) != EMPTY_STR && os.Getenv(WS_PASSWORD) != EMPTY_STR {
		username, password, ok := r.BasicAuth()
		if !ok {
			g.Log(NO_CREDS_FOR_POST)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if username != os.Getenv(WS_LOGIN) || password != os.Getenv(WS_PASSWORD) {
			g.Log(BAD_CREDS_FOR_POST)
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}

	m := Message{}
	if err := m.unmarshal(r.Body); err != nil {
		g.Log(ERR_TEMPLATE, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	g.Log(NEW_MSG_CREATED, m.toMap(true))

	g.schedule(func() {
		g.mu.Lock()
		defer g.mu.Unlock()

		g.queueMessage(m)
		g.sendMessages(m.Key)
	})

	w.WriteHeader(http.StatusAccepted)
}

func (g *Goctopus) handleGet(w http.ResponseWriter, r *http.Request) {
	defer g.mu.Unlock()
	key := r.URL.Query().Get(KEY)
	var data []byte
	g.mu.Lock()
	if key == EMPTY_STR {
		g.Log(GET_ALL_MSGS)
		b, err := g.getMarshalledKeys()
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		data = b
	} else {
		g.Log(GET_MSGS_FROM_KEY, key)
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
	defer g.mu.Unlock()
	key := r.URL.Query().Get(KEY)
	id_ := r.URL.Query().Get(ID)

	// write correct and pretty log message for incomming request
	m := MESSAGE
	if key == EMPTY_STR && id_ == EMPTY_STR {
		m = ALL + m + MULTIPLE + IN_STORAGE
	} else {
		if id_ == EMPTY_STR {
			m = fmt.Sprintf(ALL + m + MULTIPLE)
		} else {
			m = fmt.Sprintf(m+WITH_ID, id_)
		}
		if key != EMPTY_STR {
			m = fmt.Sprintf(m+FROM_KEY, key)
		}
	}
	g.Log(DELETE_METHOD + m)

	if id_ != EMPTY_STR && key == EMPTY_STR {
		g.Log(ID_BUT_NO_KEY_ERR, id_)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	g.mu.Lock()
	if key == EMPTY_STR {
		keys, err := g.storage.GetKeys()
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, key := range keys {
			g.deleteMsgQueue(key)
		}
		g.Log(ALL_DELETED)
	} else {
		if id_ == EMPTY_STR {
			g.deleteMsgQueue(key)
			g.Log(ALL_DELETED_FROM_KEY, key)
		} else {
			id, err := uuid.Parse(id_)
			if err != nil {
				g.Log(INVALID_UUID, id)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			err = g.deleteMsgById(key, id)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			g.Log(DELETED_MSG, id, key)
		}
	}
}

func (g *Goctopus) handleMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	g.Log(METHOD_NOT_ALLOWED, r.Method)
	w.WriteHeader(http.StatusMethodNotAllowed)
}
