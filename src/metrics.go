package main

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

// metrics holds lightweight runtime counters exposed in the Prometheus text
// exposition format on /metrics. No external dependency is required.
type metrics struct {
	received  atomic.Uint64
	delivered atomic.Uint64
	expired   atomic.Uint64
	authFail  atomic.Uint64
}

func (g *Goctopus) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// handleReadyz reports ready once the storage and authorizer are initialized.
func (g *Goctopus) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if g.storage == nil || g.authorizer == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ready"))
}

func (g *Goctopus) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// Gauges are computed from current state under the lock.
	g.mu.Lock()
	conns := 0
	for _, c := range g.Conns {
		conns += len(c)
	}
	keys := 0
	if k, err := g.storage.GetKeys(); err == nil {
		keys = len(k)
	}
	g.mu.Unlock()

	w.Header().Set(CONTENT_TYPE, "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP goctopus_messages_received_total Messages accepted from backends.\n")
	fmt.Fprintf(w, "# TYPE goctopus_messages_received_total counter\n")
	fmt.Fprintf(w, "goctopus_messages_received_total %d\n", g.metrics.received.Load())
	fmt.Fprintf(w, "# HELP goctopus_messages_delivered_total Messages delivered to websocket clients.\n")
	fmt.Fprintf(w, "# TYPE goctopus_messages_delivered_total counter\n")
	fmt.Fprintf(w, "goctopus_messages_delivered_total %d\n", g.metrics.delivered.Load())
	fmt.Fprintf(w, "# HELP goctopus_messages_expired_total Messages dropped because they expired.\n")
	fmt.Fprintf(w, "# TYPE goctopus_messages_expired_total counter\n")
	fmt.Fprintf(w, "goctopus_messages_expired_total %d\n", g.metrics.expired.Load())
	fmt.Fprintf(w, "# HELP goctopus_auth_failures_total Failed authentication attempts.\n")
	fmt.Fprintf(w, "# TYPE goctopus_auth_failures_total counter\n")
	fmt.Fprintf(w, "goctopus_auth_failures_total %d\n", g.metrics.authFail.Load())
	fmt.Fprintf(w, "# HELP goctopus_connections Current number of websocket connections.\n")
	fmt.Fprintf(w, "# TYPE goctopus_connections gauge\n")
	fmt.Fprintf(w, "goctopus_connections %d\n", conns)
	fmt.Fprintf(w, "# HELP goctopus_keys Current number of keys with queued messages.\n")
	fmt.Fprintf(w, "# TYPE goctopus_keys gauge\n")
	fmt.Fprintf(w, "goctopus_keys %d\n", keys)
}
