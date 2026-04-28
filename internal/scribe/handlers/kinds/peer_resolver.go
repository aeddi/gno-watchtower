package kinds

import "regexp"

// PeerResolver maps a raw peer log string to the validator subject moniker.
// Returns "" when the peer's node_id is not in the map.
type PeerResolver interface {
	Resolve(peer string) string
}

// MapResolver resolves peers via a static {node_id: subject} table loaded from
// scribe.toml's [peers] section.
type MapResolver struct{ byNodeID map[string]string }

// NewMapResolver builds a resolver. A nil map is permitted; Resolve will
// always return "".
func NewMapResolver(byNodeID map[string]string) *MapResolver {
	return &MapResolver{byNodeID: byNodeID}
}

// peerNodeIDRE captures a gnoland node_id (bech32 g1-prefixed) anywhere in the
// peer string. Matches both the new `Peer{MConn{ip:port} <node_id> dir}` log
// format and the legacy `<node_id>@<ip>:<port>` form. IPs contain only digits
// and dots so the `g1` prefix can't accidentally match them.
var peerNodeIDRE = regexp.MustCompile(`g1[a-z0-9]+`)

// Resolve extracts the node_id from a peer log blob and returns the matching
// subject. Returns "" for nil receiver, unparseable input, or unknown node_id.
func (r *MapResolver) Resolve(peer string) string {
	if r == nil || len(r.byNodeID) == 0 {
		return ""
	}
	id := peerNodeIDRE.FindString(peer)
	if id == "" {
		return ""
	}
	return r.byNodeID[id]
}

// ExtractNodeID returns the node_id embedded in a peer log blob, or "" if none.
// Exported so handlers can also clean up `peer_id` payload field consistently.
func ExtractNodeID(peer string) string {
	return peerNodeIDRE.FindString(peer)
}
