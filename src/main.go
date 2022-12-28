package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

var settings string

func main() {
	usage := "Filename"
	flag.StringVar(&settings, "file", "goctopus.yaml", usage)
	flag.StringVar(&settings, "f", "goctopus.yaml", usage+" (shortcut)")
	flag.Parse()

	app := Goctopus{AuthorizationHandler: Authorize}
	app.Start(settings)
	if err := http.ListenAndServe(fmt.Sprintf("%s:%s", app.Hostname, app.Port), &app); err != nil {
		log.Fatal(err)
	}
}
