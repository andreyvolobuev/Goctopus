package main

import (
	"path"
	"strings"
)

// Wildcard topics: a connection may be registered under a pattern key (e.g.
// "org.*") returned by the authorizer. Backends still POST to concrete keys
// (e.g. "org.sales"); the message is then delivered to every connection whose
// registered key matches that concrete key — exactly, or as a glob pattern.
//
// Matching uses path.Match semantics ("*", "?", "[...]"), where "*" does not
// cross the '/' separator.

// hasWildcard reports whether a key contains glob metacharacters.
func hasWildcard(key string) bool {
	return strings.ContainsAny(key, "*?[")
}

// keyMatches reports whether a (possibly wildcard) subscription pattern matches
// a concrete message key.
func keyMatches(pattern, key string) bool {
	if !hasWildcard(pattern) {
		return pattern == key
	}
	ok, err := path.Match(pattern, key)
	return err == nil && ok
}
