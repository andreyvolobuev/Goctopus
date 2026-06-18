package main

import (
	"net/http"
)

type Authorizer interface {
	Authorize(*Goctopus, *http.Request) ([]string, error)
	Init(*Config) error
}

// Authorizers maps an engine name to a constructor so each Goctopus instance
// gets its own authorizer. Add your custom authorizer here:
var Authorizers = map[string]func() Authorizer{
	DEFAULT: func() Authorizer { return &ProxyAuthorizer{} },
	PROXY:   func() Authorizer { return &ProxyAuthorizer{} },
	DUMMY:   func() Authorizer { return &DummyAuthorizer{} },
}
