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

	fmt.Println("---------------------------------")
	fmt.Printf("Goctopus listens to: %s:%s\n", host, port)
	fmt.Printf("Num workers is: %s\n", os.Getenv("WS_WORKERS"))
	fmt.Printf("Default message expiry is: %s\n", os.Getenv("WS_MSG_EXPIRE"))
	fmt.Printf("---------------------------------\n\n")

	app := Goctopus{}
	app.Start()
	if err := http.ListenAndServe(fmt.Sprintf("%s:%s", host, port), &app); err != nil {
		log.Fatal(err)
	}
}
