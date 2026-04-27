package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/store"
)

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

func (s *Server) handleEventsImpl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method_not_allowed", "", "")
		return
	}
	q := r.URL.Query()
	from, err := parseTime(q.Get("from"))
	if err != nil {
		writeError(w, 400, "bad_from", err.Error(), "")
		return
	}
	to, err := parseTime(q.Get("to"))
	if err != nil {
		writeError(w, 400, "bad_to", err.Error(), "")
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	query := store.EventQuery{
		ClusterID: s.deps.ClusterID,
		Subject:   q.Get("subject"),
		Kind:      q.Get("kind"),
		From:      from, To: to, Limit: limit, Cursor: q.Get("cursor"),
	}
	if v := q.Get("severity"); v != "" {
		query.Severity = strings.Split(v, ",")
	}
	if v := q.Get("state"); v != "" {
		query.State = v
	}
	evs, next, err := s.deps.Store.QueryEvents(r.Context(), query)
	if err != nil {
		writeError(w, 500, "store_error", err.Error(), "")
		return
	}
	writeJSON(w, 200, map[string]any{"events": evs, "next_cursor": next})
}
