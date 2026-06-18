package main

import (
	"encoding/json"
	"errors"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"sync"
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

	cacheTTL time.Duration
	clock    func() time.Time
	mu       sync.Mutex
	cache    map[uint64]authCacheEntry
	puts     int
}

type authCacheEntry struct {
	keys []string
	exp  time.Time
}

// cacheKey identifies a request by the credentials that determine identity
// (cookies and Authorization header), not by URL.
func cacheKey(r *http.Request) (uint64, bool) {
	cookie := r.Header.Get("Cookie")
	authz := r.Header.Get("Authorization")
	if cookie == EMPTY_STR && authz == EMPTY_STR {
		return 0, false
	}
	h := fnv.New64a()
	h.Write([]byte(cookie))
	h.Write([]byte{0})
	h.Write([]byte(authz))
	return h.Sum64(), true
}

func (a *ProxyAuthorizer) cacheGet(r *http.Request) ([]string, bool) {
	k, ok := cacheKey(r)
	if !ok {
		return nil, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	e, ok := a.cache[k]
	if !ok || a.clock().After(e.exp) {
		return nil, false
	}
	return e.keys, true
}

func (a *ProxyAuthorizer) cachePut(r *http.Request, keys []string) {
	k, ok := cacheKey(r)
	if !ok {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cache[k] = authCacheEntry{keys: keys, exp: a.clock().Add(a.cacheTTL)}

	// Periodically evict expired entries so the cache doesn't grow without
	// bound as distinct credentials come and go.
	a.puts++
	if a.puts%256 == 0 {
		now := a.clock()
		for key, e := range a.cache {
			if now.After(e.exp) {
				delete(a.cache, key)
			}
		}
	}
}

func (a *ProxyAuthorizer) Authorize(g *Goctopus, r *http.Request) ([]string, error) {
	if a.cacheTTL > 0 {
		if keys, ok := a.cacheGet(r); ok {
			return keys, nil
		}
	}

	r.URL = a.url
	r.RequestURI = EMPTY_STR

	resp, err := a.client.Do(r)
	if err != nil {
		g.Log(ERR_TEMPLATE, err)
		return nil, err
	}
	defer resp.Body.Close() // avoid leaking the connection/fd on every auth call

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
	if a.cacheTTL > 0 {
		a.cachePut(r, keys)
	}
	return keys, nil
}

func (a *ProxyAuthorizer) Init(cfg *Config) error {
	AuthURL, err := url.Parse(cfg.AuthURL)
	if err != nil {
		return err
	}
	a.url = AuthURL

	// Bound the auth call so a hung auth backend can't pin a worker forever.
	a.client = &http.Client{Timeout: cfg.AuthTimeout}

	a.cacheTTL = cfg.AuthCacheTTL
	a.clock = time.Now
	a.cache = make(map[uint64]authCacheEntry)
	return nil
}
