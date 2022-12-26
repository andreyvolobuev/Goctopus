package main

import (
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

func (g *Goctopus) LoadSettings(filename string) {

	if filename == "" {
		filename = "goctopus.yaml"
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		log.Fatal(err)
	}

	if err := yaml.Unmarshal(data, g); err != nil {
		log.Fatal(err)
	}
}
