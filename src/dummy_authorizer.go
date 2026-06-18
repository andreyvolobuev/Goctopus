package main

import "net/http"

// DummyAuthorizer authorizes every connection under a single key captured from
// the configured auth URL value at Init time. Intended for development only.
// The key may be a wildcard pattern (e.g. "org.*").
type DummyAuthorizer struct {
	keys []string
}

func (d *DummyAuthorizer) Authorize(g *Goctopus, r *http.Request) ([]string, error) {
	return d.keys, nil
}

func (d *DummyAuthorizer) Init(cfg *Config) error {
	d.keys = []string{cfg.AuthURL}
	return nil
}
