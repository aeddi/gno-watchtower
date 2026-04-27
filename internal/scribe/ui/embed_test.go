package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesIndex(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content-type = %q", ct)
	}
	if !strings.Contains(w.Body.String(), "scribe") {
		t.Errorf("expected body to contain 'scribe', got: %s", w.Body.String())
	}
}

func TestHandlerSPAFallsBackToIndex(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/this-route-does-not-exist-on-disk", nil)
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Errorf("SPA fallback status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `<div id="app"></div>`) {
		t.Errorf("SPA fallback didn't return index.html shell")
	}
}
