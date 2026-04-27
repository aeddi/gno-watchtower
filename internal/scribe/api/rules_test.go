package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeddi/gno-watchtower/internal/scribe/analysis"
)

type stubRule struct{}

func (stubRule) Meta() analysis.Meta {
	return analysis.Meta{
		Code: "stub", Version: 1, Severity: analysis.SeverityWarning,
		Kinds: []string{"validator.x"}, Description: "stub for tests",
	}
}

func (stubRule) Evaluate(_ context.Context, _ analysis.Trigger, _ analysis.Deps, _ analysis.Emitter) {
}

func TestApiRulesListsRegisteredRules(t *testing.T) {
	analysis.ResetRegistryForTest(t)
	analysis.Register(stubRule{}, "# stub\n## What it detects\nstub.\n")

	srv := New(Deps{}).Handler()
	r := httptest.NewRequest(http.MethodGet, "/api/rules", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	var got []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rules, want 1", len(got))
	}
	if got[0]["kind"] != "diagnostic.stub_v1" {
		t.Errorf("kind = %v", got[0]["kind"])
	}
	if got[0]["doc_ref"] != "/docs/rules/diagnostic.stub_v1" {
		t.Errorf("doc_ref = %v", got[0]["doc_ref"])
	}
}

func TestApiDocsRulesServesEmbeddedMarkdown(t *testing.T) {
	analysis.ResetRegistryForTest(t)
	doc := "# stub_v1\n## What it detects\nthe doc.\n"
	analysis.Register(stubRule{}, doc)

	srv := New(Deps{}).Handler()
	r := httptest.NewRequest(http.MethodGet, "/docs/rules/diagnostic.stub_v1", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Errorf("content-type = %q", ct)
	}
	if w.Body.String() != doc {
		t.Errorf("body mismatch: %q vs %q", w.Body.String(), doc)
	}
}

func TestApiDocsRulesUnknownReturns404(t *testing.T) {
	analysis.ResetRegistryForTest(t)
	srv := New(Deps{}).Handler()
	r := httptest.NewRequest(http.MethodGet, "/docs/rules/missing_v1", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
