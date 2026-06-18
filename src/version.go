package main

import (
	"encoding/json"
	"net/http"
	"runtime"
)

// Build information. version and commit are overridable at build time:
//
//	go build -ldflags "-X main.version=1.2.3 -X main.commit=$(git rev-parse --short HEAD)"
var (
	version = "dev"
	commit  = "none"
)

func (g *Goctopus) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(CONTENT_TYPE, APPLICATION_JSON)
	json.NewEncoder(w).Encode(map[string]string{
		"version": version,
		"commit":  commit,
		"go":      runtime.Version(),
	})
}
