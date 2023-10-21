package main

import (
	"net/http"
)

type Authorizer interface {
	Authorize(*Goctopus, *http.Request) ([]string, error)
}

// This whole module might be redefined in order to create authorization logic
// that you actually need in your own project.
//
// This one is pretty straight-forward and just does forwarding of requests
// to URL that is defined in WS_AUTH_URL environment variable OR that is set
// via --auth flag when you run the app.
//
// The URL must response with some kind of object, structure of which is described
// bellow, in AuthResponse struct. AuthResponse implements Export method, that
// returns a list of strings. Those are identifiers of the request.

// Authentication response schema shall be described into this struct
type AuthResponse struct {
	User struct {
		Email        string `json:"email"`
		Ogranization string `json:"organization_name"`
	}
}

// Export actually needed fields of Authentication Request fields as a list of strings
func (r *AuthResponse) Export() []string {
	exported := []string{}

	keys := []string{r.User.Email, r.User.Ogranization}
	for _, key := range keys {
		if key != "" {
			exported = append(exported, key)
		}
	}

	return exported
}

func (g *Goctopus) Authorize(r *http.Request) ([]string, error) {
	// AuthURL, err := url.Parse(os.Getenv("WS_AUTH_URL"))
	// if err != nil {
	// 	g.Log("%s", err)
	// }

	// r.URL = AuthURL
	// r.RequestURI = ""

	// client := &http.Client{}
	// resp, err := client.Do(r)
	// if err != nil {
	// 	g.Log("%s", err)
	// 	return []string{}, err
	// }

	// b, err := io.ReadAll(resp.Body)
	// if err != nil {
	// 	g.Log("%s", err)
	// 	return []string{}, err
	// }

	// data := AuthResponse{}
	// if err := json.Unmarshal(b, &data); err != nil {
	// 	g.Log("%s", err)
	// 	return []string{}, err
	// }

	// keys := data.Export()
	// if len(keys) == 0 {
	// 	return []string{}, errors.New("invalid credentials")
	// }
	keys := []string{"a"}
	return keys, nil
}

var dummy = DummyAuthorizer{keys: []string{"test"}}
var Authorizers = map[string]Authorizer{
	"dummy":   &dummy,
	"default": &dummy,
}
