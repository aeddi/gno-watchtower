package handlers

import "testing"

// TestMetaAccessorsReturnNonEmpty enumerates registered kinds and verifies
// each one's Meta() returns non-empty Kind/Source/Description/DocRef. The
// previous version called per-handler constructors directly; after the
// one-file-per-kind split (Phase 9.3) those constructors live in the
// kinds package, so we now go through the registry instead.
func TestMetaAccessorsReturnNonEmpty(t *testing.T) {
	for _, k := range RegisteredKinds() {
		h := NewHandler(k, "c1")
		if h == nil {
			t.Errorf("NewHandler(%q) returned nil", k)
			continue
		}
		m := h.Meta()
		if m.Kind == "" || m.Source == "" || m.Description == "" || m.DocRef == "" {
			t.Errorf("handler %s: empty Meta field: %+v", h.Name(), m)
		}
		if m.DocRef != "/docs/handlers/"+m.Kind {
			t.Errorf("handler %s: DocRef = %q, want /docs/handlers/%s", h.Name(), m.DocRef, m.Kind)
		}
	}
}
