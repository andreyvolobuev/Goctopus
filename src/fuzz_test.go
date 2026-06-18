package main

import "testing"

// FuzzMessageUnmarshal ensures parsing arbitrary POST bodies never panics.
func FuzzMessageUnmarshal(f *testing.F) {
	f.Add(`{"key":"k","value":{"a":1},"expire":"5m"}`)
	f.Add(`{"keys":["a","b"],"value":1,"message_id":"11111111-1111-1111-1111-111111111111"}`)
	f.Add(`{"value":1}`)
	f.Add(`not json`)
	f.Add(``)

	f.Fuzz(func(t *testing.T, data string) {
		m := Message{}
		_ = m.unmarshal(body(data), "30m") // must not panic
	})
}
