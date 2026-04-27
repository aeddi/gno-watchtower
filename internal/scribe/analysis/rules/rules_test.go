package rules

import (
	"os"
	"strings"
	"testing"

	"github.com/aeddi/gno-watchtower/internal/scribe/analysis"
)

var requiredSections = []string{
	"## What it detects",
	"## Detection logic",
	"## Common causes",
	"## Remediation / workaround",
	"## Linked signals",
}

// TestEveryRuleHasCompleteDoc verifies every registered analysis rule's
// markdown contains all required sections and no leftover TODO markers.
// Runs as part of `go test ./internal/scribe/analysis/rules/...` in CI.
func TestEveryRuleHasCompleteDoc(t *testing.T) {
	codes := analysis.RegisteredCodes()
	if len(codes) < 5 {
		t.Fatalf("expected ≥ 5 registered v1 rules, got %d: %v", len(codes), codes)
	}
	for _, kind := range codes {
		doc := analysis.GetDoc(kind)
		if strings.Contains(doc, "TODO") {
			t.Errorf("rule %s: doc contains TODO markers", kind)
		}
		for _, section := range requiredSections {
			if !strings.Contains(doc, section) {
				t.Errorf("rule %s: doc missing required section %q", kind, section)
			}
		}
	}
}

// TestEveryRecoveryRuleReferencesRecoveryKey enforces the rehydration
// contract: every rule that exposes RecoveryTracker() must reference
// "recovery_key" in its sibling _test.go file. The check is structural —
// it cannot prove the field is actually written on every open emission,
// but it ensures contributors writing a new recovery rule remember the
// convention.
func TestEveryRecoveryRuleReferencesRecoveryKey(t *testing.T) {
	for _, kind := range analysis.RegisteredCodes() {
		r := analysis.Lookup(kind)
		if r == nil {
			continue
		}
		if _, ok := r.(interface {
			RecoveryTracker() *analysis.Tracker
		}); !ok {
			continue
		}
		// Convert "diagnostic.<slug>_v<N>" → "<slug>" for the filename lookup.
		name := strings.TrimPrefix(kind, "diagnostic.")
		if i := strings.LastIndex(name, "_v"); i > 0 {
			name = name[:i]
		}
		body, err := os.ReadFile(name + "_test.go")
		if err != nil {
			t.Errorf("rule %s: cannot read sibling test file %s_test.go: %v", kind, name, err)
			continue
		}
		if !strings.Contains(string(body), `"recovery_key"`) {
			t.Errorf("rule %s: %s_test.go must reference \"recovery_key\" payload field", kind, name)
		}
	}
}
