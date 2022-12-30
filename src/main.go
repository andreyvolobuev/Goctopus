package main

import (
	"fmt"
	"log"
	"os"
	"net/http"
)

var settings string

func main() {
	host := os.Getenv("WS_HOST")
	port := os.Getenv("WS_PORT")

	app := Goctopus{}
	app.Start()
	if err := http.ListenAndServe(fmt.Sprintf("%s:%s", host, port), &app); err != nil {
		log.Fatal(err)
	}
}
