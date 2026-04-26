package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aeddi/gno-watchtower/internal/scribe/store"
)

func (s *Server) handleEventsStreamImpl(w http.ResponseWriter, r *http.Request) {
	if s.deps.Writer == nil {
		writeError(w, 503, "writer_unavailable", "", "")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "no_flusher", "", "")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Optional: replay since Last-Event-ID.
	if since := r.Header.Get("Last-Event-ID"); since != "" {
		evs, _, _ := s.deps.Store.QueryEvents(r.Context(), store.EventQuery{
			ClusterID: s.deps.ClusterID, Cursor: since, Limit: 1000,
		})
		for _, ev := range evs {
			b, _ := json.Marshal(ev)
			fmt.Fprintf(w, "id: %s\ndata: %s\n\n", ev.EventID, string(b))
		}
		flusher.Flush()
	}

	sub := s.deps.Writer.Subscribe(64)
	defer s.deps.Writer.Unsubscribe(sub)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub:
			if !ok {
				return
			}
			b, _ := json.Marshal(ev)
			fmt.Fprintf(w, "id: %s\ndata: %s\n\n", ev.EventID, string(b))
			flusher.Flush()
		}
	}
}
