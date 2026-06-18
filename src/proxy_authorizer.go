package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

type AuthResponse struct {
	User struct {
		Email        string `json:"email"`
		Ogranization string `json:"organization_name"`
	}
}

func (r *AuthResponse) Export() []string {
	exported := []string{}

	keys := []string{r.User.Email, r.User.Ogranization}
	for _, key := range keys {
		if key != EMPTY_STR {
			exported = append(exported, key)
		}
	}

	return exported
}

type ProxyAuthorizer struct {
	url    *url.URL
	client *http.Client
}

func (a *ProxyAuthorizer) Authorize(g *Goctopus, r *http.Request) ([]string, error) {
	r.URL = a.url
	r.RequestURI = EMPTY_STR

	resp, err := a.client.Do(r)
	if err != nil {
		g.Log(ERR_TEMPLATE, err)
		return nil, err
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		g.Log(ERR_TEMPLATE, err)
		return nil, err
	}

	data := AuthResponse{}
	if err := json.Unmarshal(b, &data); err != nil {
		g.Log(ERR_TEMPLATE, err)
		return nil, err
	}

	keys := data.Export()
	if len(keys) == 0 {
		return nil, errors.New(AUTH_INVALID_CREDS)
	}
	return keys, nil
}

func (a *ProxyAuthorizer) Init() error {
	AuthURL, err := url.Parse(os.Getenv(WS_AUTH_URL))
	if err != nil {
		return err
	}
	a.url = AuthURL

	// Bound the auth call so a hung auth backend can't pin a worker forever.
	timeout, err := time.ParseDuration(os.Getenv(WS_AUTH_TIMEOUT))
	if err != nil || timeout <= 0 {
		timeout = 10 * time.Second
	}
	a.client = &http.Client{Timeout: timeout}
	return nil
}
