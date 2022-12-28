package main

import (
	"errors"
	"log"
	"net/http"
)

// One might redefine this func in order to get different authorization logic,
// but what's important is that this func has to accept request and return
// list or strings that represent names for the authorized user and an error
func Authorize(r *http.Request) ([]string, error) {
	// AuthURL, err := url.Parse("http://localhost:8000/")
	// if err != nil {
	// 	log.Printf("%s\n", err)
	// }

	// r.URL = AuthURL
	// r.RequestURI = ""

	// client := &http.Client{}
	// resp, err := client.Do(r)
	// if err != nil {
	// 	log.Printf("%s\n", err)
	// 	return []string{}, err
	// }

	// b, err := io.ReadAll(resp.Body)
	// if err != nil {
	// 	log.Printf("%s\n", err)
	// 	return []string{}, err
	// }

	// data := map[string]any{}
	// if err := json.Unmarshal(b, &data); err != nil {
	// 	log.Printf("%s\n", err)
	// 	return []string{}, err
	// }

	// remove this
	em := make(map[string]string)
	data := make(map[string]any)
	em["email"] = "avvolob@gmail.com"
	data["user"] = em
	// until this

	_, ok := data["user"]
	if !ok {
		log.Printf("user credentials are missing\n")
		return []string{}, errors.New("user credentials are missing")
	}

	user := data["user"].(map[string]string)
	email := []string{user["email"]}

	return email, nil
}
