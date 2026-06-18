package main

import (
	"crypto/subtle"
	"fmt"
	"net/http"

	"github.com/gobwas/ws"
	"github.com/google/uuid"
)

func (g *Goctopus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case ROOT:
		g.handleHTTP(w, r)

	case WS, WS_:
		g.handleWs(w, r)

	case HEALTHZ:
		g.handleHealthz(w, r)

	case READYZ:
		g.handleReadyz(w, r)

	case METRICS:
		g.handleMetrics(w, r)

	case VERSION:
		g.handleVersion(w, r)

	default:
		http.NotFound(w, r)
	}
}

func (g *Goctopus) handleHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(CONTENT_TYPE, APPLICATION_JSON)

	if !g.allow(r) {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}

	// The whole JSON API (publish, list keys, read and delete messages) is a
	// backend-only surface: it must be authenticated, not just POST. Listing
	// keys leaks user identifiers and DELETE can wipe every queue.
	if !g.authorizeBackend(w, r) {
		return
	}

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
	if !g.allow(r) {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}

	// Reject cross-origin upgrades (CSWSH) before doing any work.
	if origin := r.Header.Get(ORIGIN); !g.config.originAllowed(origin) {
		g.Log(ORIGIN_REJECTED, origin)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	keys, err := g.authorizer.Authorize(g, r)

	if err != nil {
		g.Log(AUTH_FAILED, err)
		g.metrics.authFail.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		g.Log(ERR_TEMPLATE, err)
		return
	}

	c := newClient(conn, keys, g.config.WriteTimeout)

	g.mu.Lock()
	for _, key := range keys {
		g.Conns[key] = append(g.Conns[key], c)
		if hasWildcard(key) {
			g.patterns[key] = true
		}
	}
	g.mu.Unlock()
	g.Log(SAVED_NEW_CONN, keys)

	go g.readLoop(c)
	go g.pingLoop(c)

	g.schedule(func() {
		for _, key := range keys {
			if hasWildcard(key) {
				// Flush the backlog of every concrete key this pattern covers.
				g.mu.Lock()
				matched := g.storageKeysMatching(key)
				g.mu.Unlock()
				for _, k := range matched {
					g.sendMessages(k)
				}
			} else {
				g.sendMessages(key)
			}
		}
	})
}

func (g *Goctopus) handlePost(w http.ResponseWriter, r *http.Request) {
	g.Log(POST_NEW_MSG)

	r.Body = http.MaxBytesReader(w, r.Body, g.config.MaxMessageBytes)

	m := Message{}
	if err := m.unmarshal(r.Body, g.config.DefaultExpire); err != nil {
		g.Log(ERR_TEMPLATE, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	g.Log(NEW_MSG_CREATED, m.toMap(true))
	g.metrics.received.Add(1)

	g.schedule(func() {
		g.queueMessage(m)
		g.notify(m.Key)
		g.sendMessages(m.Key)
	})

	w.WriteHeader(http.StatusAccepted)
}

// authorizeBackend enforces Basic Auth for the backend JSON API (publish, list,
// read, delete). It fails closed: unless InsecureNoAuth is explicitly enabled,
// missing or empty credentials cause the request to be rejected rather than
// silently accepted.
func (g *Goctopus) authorizeBackend(w http.ResponseWriter, r *http.Request) bool {
	if g.config.InsecureNoAuth {
		return true
	}

	login := g.config.Login
	pass := g.config.Password
	if login == EMPTY_STR || pass == EMPTY_STR {
		g.Log(NO_CREDS_FOR_POST)
		g.metrics.authFail.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}

	username, password, ok := r.BasicAuth()
	if !ok {
		g.Log(NO_CREDS_FOR_POST)
		g.metrics.authFail.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}

	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(login)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(password), []byte(pass)) == 1
	if !userOK || !passOK {
		g.Log(BAD_CREDS_FOR_POST)
		g.metrics.authFail.Add(1)
		w.WriteHeader(http.StatusForbidden)
		return false
	}

	return true
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

	// Validate the id before taking the lock so a bad request never holds it.
	var id uuid.UUID
	if id_ != EMPTY_STR {
		parsed, err := uuid.Parse(id_)
		if err != nil {
			g.Log(INVALID_UUID, id_)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		id = parsed
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	switch {
	case key == EMPTY_STR:
		keys, err := g.storage.GetKeys()
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, k := range keys {
			g.deleteMsgQueue(k)
		}
		g.Log(ALL_DELETED)

	case id_ == EMPTY_STR:
		g.deleteMsgQueue(key)
		g.Log(ALL_DELETED_FROM_KEY, key)

	default:
		// Caller holds g.mu, so use the *Locked variant to avoid a deadlock.
		if err := g.deleteMsgByIdLocked(key, id); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		g.Log(DELETED_MSG, id, key)
	}
}

func (g *Goctopus) handleMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	g.Log(METHOD_NOT_ALLOWED, r.Method)
	w.WriteHeader(http.StatusMethodNotAllowed)
}
