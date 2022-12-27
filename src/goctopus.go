package main

import (
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

type Goctopus struct {
	Queue    []Message
	Conns    []byte
	Hostname string `yaml:"hostname"`
	Port     string `yaml:"port"`
}

func (g *Goctopus) LoadSettings(filename string) {

	if filename == "" {
		filename = "goctopus.yaml"
	}

	data, err := os.ReadFile(filename)
	fmt.Println(string(data))
	if err != nil {
		log.Fatal(err)
	}

	if err := yaml.Unmarshal(data, g); err != nil {
		log.Fatal(err)
	}
}
