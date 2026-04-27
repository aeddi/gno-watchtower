package handlers

import "testing"

// TestMetaAccessorsReturnNonEmpty confirms each handler's Meta() returns
// non-empty Kind/Source/Description/DocRef.
func TestMetaAccessorsReturnNonEmpty(t *testing.T) {
	cluster := "c1"
	cases := []Handler{
		NewHeight(cluster), NewOnline(cluster), NewPeers(cluster),
		NewMempool(cluster), NewVotingPower(cluster), NewValsetSize(cluster),
		NewProposed(cluster), NewConsensusRoundStep(cluster), NewVoteCast(cluster),
		NewPeerConnected(cluster), NewPeerDisconnected(cluster),
		NewBlockCommitted(cluster), NewValsetChanged(cluster), NewTxExecuted(cluster),
	}
	for _, h := range cases {
		m := h.Meta()
		if m.Kind == "" || m.Source == "" || m.Description == "" || m.DocRef == "" {
			t.Errorf("handler %s: empty Meta field: %+v", h.Name(), m)
		}
		if m.DocRef != "/docs/handlers/"+m.Kind {
			t.Errorf("handler %s: DocRef = %q, want /docs/handlers/%s", h.Name(), m.DocRef, m.Kind)
		}
	}
}
