package main

import "net/http"

type DummyAuthorizer struct {
	keys []string
}

func (d *DummyAuthorizer) Authorize(g *Goctopus, r *http.Request) ([]string, error) {
	return d.keys, nil
}
