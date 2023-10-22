package main

import (
	"net/http"
)

type Authorizer interface {
	Authorize(*Goctopus, *http.Request) ([]string, error)
	Init() error
}

var proxy = ProxyAuthorizer{}
var Authorizers = map[string]Authorizer{
	DEFAULT: &proxy,
	PROXY:   &proxy,
	DUMMY:   &DummyAuthorizer{},
}
