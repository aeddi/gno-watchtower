package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "github.com/aeddi/gno-watchtower/internal/scribe/handlers/kinds" // register the 14 v1 handlers
)

func TestApiHandlersListsRegisteredKinds(t *testing.T) {
	srv := New(Deps{}).Handler()
	r := httptest.NewRequest(http.MethodGet, "/api/handlers", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var got []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) < 14 {
		t.Errorf("want ≥14 handlers, got %d: %+v", len(got), got)
	}
	for _, h := range got {
		if h["kind"] == "" || h["source"] == "" || h["doc_ref"] == "" {
			t.Errorf("handler entry missing fields: %+v", h)
		}
	}
}

func TestApiDocsHandlersServesEmbeddedMarkdown(t *testing.T) {
	srv := New(Deps{}).Handler()
	r := httptest.NewRequest(http.MethodGet, "/docs/handlers/validator.height_advanced", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct == "" {
		t.Errorf("missing content-type")
	}
}

func TestApiDocsHandlersUnknownReturns404(t *testing.T) {
	srv := New(Deps{}).Handler()
	r := httptest.NewRequest(http.MethodGet, "/docs/handlers/totally.bogus.kind", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
