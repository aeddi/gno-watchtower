package api

import (
	"net/http"
	"strings"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers"
)

// handleHandlersImpl implements GET /api/handlers — returns a JSON list of
// every registered event-kind handler with its descriptive metadata.
func (s *Server) handleHandlersImpl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method_not_allowed", "", "")
		return
	}
	kinds := handlers.RegisteredKinds()
	out := make([]map[string]any, 0, len(kinds))
	for _, k := range kinds {
		// Cluster ID is irrelevant for descriptive metadata; pass empty.
		h := handlers.NewHandler(k, "")
		if h == nil {
			continue
		}
		m := h.Meta()
		out = append(out, map[string]any{
			"kind":        m.Kind,
			"source":      string(m.Source),
			"description": m.Description,
			"doc_ref":     m.DocRef,
		})
	}
	writeJSON(w, 200, out)
}

// handleDocsHandlersImpl implements GET /docs/handlers/<kind>. The trailing
// path segment is the registry key (e.g. "validator.height_advanced").
func (s *Server) handleDocsHandlersImpl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method_not_allowed", "", "")
		return
	}
	kind := strings.TrimPrefix(r.URL.Path, "/docs/handlers/")
	if kind == "" || strings.ContainsRune(kind, '/') {
		writeError(w, 400, "bad_path", "expected /docs/handlers/<kind>", "")
		return
	}
	doc := handlers.GetHandlerDoc(kind)
	if doc == "" {
		writeError(w, 404, "not_found", "no such handler "+kind, "")
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(200)
	_, _ = w.Write([]byte(doc))
}
