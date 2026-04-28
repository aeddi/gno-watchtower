package kinds_test

import (
	"testing"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers/kinds"
)

func TestMapResolver_Resolve(t *testing.T) {
	r := kinds.NewMapResolver(map[string]string{
		"g1ftqcp3446jkfqnnctpvvsjanf42fcrrf94z7u4": "node-1",
		"g14nn6jgzzp78fajeug4v8kmu75vsqj05z9q5jdd": "node-9",
	})
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"out connection", "Peer{MConn{192.168.155.5:26656} g1ftqcp3446jkfqnnctpvvsjanf42fcrrf94z7u4 out}", "node-1"},
		{"in connection", "Peer{MConn{192.168.155.11:56568} g14nn6jgzzp78fajeug4v8kmu75vsqj05z9q5jdd in}", "node-9"},
		{"unknown node_id", "Peer{MConn{10.0.0.1:1234} g1nobody4tnv9wxyz in}", ""},
		{"malformed", "not a peer string", ""},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := r.Resolve(tc.in); got != tc.want {
				t.Fatalf("Resolve(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMapResolver_Nil(t *testing.T) {
	var r *kinds.MapResolver
	if got := r.Resolve("Peer{MConn{1:2} g1abc out}"); got != "" {
		t.Fatalf("nil resolver should return empty, got %q", got)
	}
}
