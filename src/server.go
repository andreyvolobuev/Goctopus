package main

import (
	"fmt"
	"net/http"
)

func (g *Goctopus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Println(r.Method)
	fmt.Println(r.Header)
	fmt.Println(r.Body)
}
