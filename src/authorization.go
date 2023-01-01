package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
)

// Authentication response schema shall be described into this struct
type AuthResponse struct {
	User struct {
		Email        string `json:"email"`
		Ogranization string `json:"organization_name"`
	}
}

// Export actually needed fields of Authentication Request fields as a list of strings
func (r *AuthResponse) Export() []string {
	return []string{r.User.Email, r.User.Ogranization}
}

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

	data := AuthResponse{}
	if err := json.Unmarshal(b, &data); err != nil {
		log.Printf("%s\n", err)
		return []string{}, err
	}

	keys := data.Export()
	if len(keys) == 0 {
		return []string{}, errors.New("invalid credentials")
	}
	return keys, nil
}
