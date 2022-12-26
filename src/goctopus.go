package main

type Goctopus struct {
	Storage map[string]string

	Hostname string `yaml:"hostname"`
	Port     string `yaml:"port"`
}
