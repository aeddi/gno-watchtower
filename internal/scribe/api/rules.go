package api

import (
	"net/http"
	"strings"

	"github.com/aeddi/gno-watchtower/internal/scribe/analysis"
)

// handleRulesImpl implements GET /api/rules.
func (s *Server) handleRulesImpl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method_not_allowed", "", "")
		return
	}
	codes := analysis.RegisteredCodes()
	out := make([]map[string]any, 0, len(codes))
	for _, k := range codes {
		m := analysis.GetMeta(k)
		params := map[string]any{}
		for name, spec := range m.Params {
			params[name] = spec.Default
		}
		out = append(out, map[string]any{
			"code":                m.Code,
			"version":             m.Version,
			"kind":                m.Kind(),
			"severity":            string(m.Severity),
			"kinds":               m.Kinds,
			"tick_period_seconds": int64(m.TickPeriod.Seconds()),
			"description":         m.Description,
			"doc_ref":             "/docs/rules/" + m.Kind(),
			"enabled":             true, // engine enforces; surface here for clients
			"params":              params,
		})
	}
	writeJSON(w, 200, out)
}

// handleDocsRulesImpl implements GET /docs/rules/<kind>. The trailing path
// segment is the registry key (e.g. "diagnostic.block_missed_v1").
func (s *Server) handleDocsRulesImpl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method_not_allowed", "", "")
		return
	}
	kind := strings.TrimPrefix(r.URL.Path, "/docs/rules/")
	if kind == "" || strings.ContainsRune(kind, '/') {
		writeError(w, 400, "bad_path", "expected /docs/rules/<kind>", "")
		return
	}
	doc := analysis.GetDoc(kind)
	if doc == "" {
		writeError(w, 404, "not_found", "no such rule "+kind, "")
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(200)
	_, _ = w.Write([]byte(doc))
}
