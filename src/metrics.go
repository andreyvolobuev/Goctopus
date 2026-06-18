package main

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// metrics holds the Prometheus instruments for an instance. Gauges for live
// connection/key counts are reported via callbacks at scrape time.
type metrics struct {
	registry *prometheus.Registry

	received  prometheus.Counter
	delivered prometheus.Counter
	expired   prometheus.Counter
	authFail  prometheus.Counter
	delivery  prometheus.Histogram
}

// Pinger is implemented by storage backends that can report liveness.
type Pinger interface {
	PingContext(ctx context.Context) error
}

func newMetrics(g *Goctopus) *metrics {
	reg := prometheus.NewRegistry()
	m := &metrics{
		registry: reg,
		received: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "goctopus_messages_received_total", Help: "Messages accepted from backends.",
		}),
		delivered: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "goctopus_messages_delivered_total", Help: "Messages acknowledged by websocket clients.",
		}),
		expired: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "goctopus_messages_expired_total", Help: "Messages dropped because they expired.",
		}),
		authFail: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "goctopus_auth_failures_total", Help: "Failed authentication attempts.",
		}),
		delivery: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "goctopus_delivery_seconds", Help: "Time from enqueue to delivery.",
			Buckets: prometheus.DefBuckets,
		}),
	}

	reg.MustRegister(m.received, m.delivered, m.expired, m.authFail, m.delivery)

	buildInfo := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "goctopus_build_info",
		Help:        "Build information; always 1, labelled with version and commit.",
		ConstLabels: prometheus.Labels{"version": version, "commit": commit},
	})
	buildInfo.Set(1)
	reg.MustRegister(buildInfo)

	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "goctopus_connections", Help: "Current number of websocket connections.",
	}, func() float64 {
		g.mu.Lock()
		defer g.mu.Unlock()
		n := 0
		for _, c := range g.Conns {
			n += len(c)
		}
		return float64(n)
	}))
	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "goctopus_keys", Help: "Current number of keys with queued messages.",
	}, func() float64 {
		// Storage is self-synchronized; don't hold g.mu across a (possibly
		// remote) GetKeys during a scrape.
		k, err := g.storage.GetKeys()
		if err != nil {
			return 0
		}
		return float64(len(k))
	}))

	return m
}

func (g *Goctopus) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// handleReadyz reports ready once storage and authorizer are initialized and,
// for backends that support it (Redis), the backend responds to a ping.
func (g *Goctopus) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if g.storage == nil || g.authorizer == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if p, ok := g.storage.(Pinger); ok {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := p.PingContext(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ready"))
}

func (g *Goctopus) handleMetrics(w http.ResponseWriter, r *http.Request) {
	promhttp.HandlerFor(g.metrics.registry, promhttp.HandlerOpts{}).ServeHTTP(w, r)
}
