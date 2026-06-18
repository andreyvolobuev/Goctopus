package main

import "testing"

func TestOriginAllowed(t *testing.T) {
	cases := []struct {
		allowed []string
		origin  string
		want    bool
	}{
		{nil, "https://any.example", true},                                // no restriction
		{[]string{"*"}, "https://any.example", true},                      // wildcard
		{[]string{"https://good.example"}, "", true},                      // non-browser client
		{[]string{"https://good.example"}, "https://good.example", true},  // match
		{[]string{"https://good.example"}, "https://evil.example", false}, // mismatch
		{[]string{"https://a.example", "https://b.example"}, "https://b.example", true},
	}
	for _, c := range cases {
		cfg := &Config{AllowedOrigins: c.allowed}
		if got := cfg.originAllowed(c.origin); got != c.want {
			t.Errorf("originAllowed(%v, %q) = %v, want %v", c.allowed, c.origin, got, c.want)
		}
	}
}
