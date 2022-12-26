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

	goctopus := Goctopus{}
	goctopus.LoadSettings(settings)
	if err := http.ListenAndServe(fmt.Sprintf("%s:%s", goctopus.Hostname, goctopus.Port), &goctopus); err != nil {
		log.Fatal(err)
	}
}
