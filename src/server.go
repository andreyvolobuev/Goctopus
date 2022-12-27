package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (g *Goctopus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		g._handleHTTP(w, r)
	case "/ws", "/ws/":
		g._handleWs(w, r)
	}
}

func (g *Goctopus) _handleHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		g._handleGet(w, r)

	case "POST":
		g._handlePost(w, r)

	case "DELETE":
		g._handleDelete(w, r)

	default:
		g._handleBadRequest(w, r)

	}
}

func (g *Goctopus) _handleWs(w http.ResponseWriter, r *http.Request) {

}

func (g *Goctopus) _handleGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	m := make(map[string]string)
	m["22"] = "33"
	data, err := json.Marshal(m)
	if err != nil {
		fmt.Println(err)
	}
	w.Write(data)
}

func (g *Goctopus) _handlePost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

}

func (g *Goctopus) _handleDelete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)

}

func (g *Goctopus) _handleBadRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)

}
