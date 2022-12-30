package main

import (
    "encoding/json"
    "errors"
    "io"
	"os"
    "log"
    "net/http"
    "net/url"
)

// One might redefine this func in order to get different authorization logic,
// but what's important is that this func has to accept request and return
// list or strings that represent names for the authorized user and an error
func Authorize(r *http.Request) ([]string, error) {
    AuthURL, err := url.Parse(os.Getenv("WS_AUTH_URL"))
	if err != nil {
    	log.Printf("%s\n", err)
	}

	r.URL = AuthURL
	r.RequestURI = ""

	client := &http.Client{}
	resp, err := client.Do(r)
	if err != nil {
		log.Printf("%s\n", err)
		return []string{}, err
	}

	b, err := io.ReadAll(resp.Body)

	if err != nil {
		log.Printf("%s\n", err)
		return []string{}, err
	}

	data := make(map[string]any)
	if err := json.Unmarshal(b, &data); err != nil {
		log.Printf("%s\n", err)
		return []string{}, err
	}

	user, ok := data["user"].(map[string]any)
	if !ok {
		log.Printf("user credentials are missing\n")
		return []string{}, errors.New("user credentials are missing")
	}

	email := []string{user["email"].(string)}

	return email, nil
}
