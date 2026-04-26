package api

import (
	"net/http"
	"reflect"

	"github.com/aeddi/gno-watchtower/internal/scribe/kinds"
)

func (s *Server) handleSubjectsImpl(w http.ResponseWriter, r *http.Request) {
	subs, err := s.deps.Store.ListSubjects(r.Context(), s.deps.ClusterID)
	if err != nil {
		writeError(w, 500, "store_error", err.Error(), "")
		return
	}
	writeJSON(w, 200, map[string]any{"subjects": subs})
}

func (s *Server) handleEventKindsImpl(w http.ResponseWriter, r *http.Request) {
	type entry struct {
		Kind   string         `json:"kind"`
		Schema map[string]any `json:"schema"`
	}
	out := make([]entry, 0, len(kinds.All()))
	for _, k := range kinds.All() {
		schema := map[string]any{}
		t := reflect.TypeOf(k)
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := f.Tag.Get("json")
			name := f.Name
			if tag != "" {
				name = tag
				if idx := indexComma(name); idx >= 0 {
					name = name[:idx]
				}
			}
			schema[name] = f.Type.String()
		}
		out = append(out, entry{Kind: k.Kind(), Schema: schema})
	}
	writeJSON(w, 200, map[string]any{"kinds": out})
}

func indexComma(s string) int {
	for i := range s {
		if s[i] == ',' {
			return i
		}
	}
	return -1
}
