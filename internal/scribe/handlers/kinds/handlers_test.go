package kinds_test

import (
	"strings"
	"testing"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers"
)

var requiredHandlerSections = []string{
	"## What it represents",
	"## How it's detected",
	"## Linked source code",
	"## Payload fields",
}

// TestEveryHandlerHasCompleteDoc verifies that every registered handler's
// markdown doc contains all required sections and has no leftover TODO
// markers. Runs as part of `go test ./...` in CI.
func TestEveryHandlerHasCompleteDoc(t *testing.T) {
	codes := handlers.RegisteredKinds()
	if len(codes) < 14 {
		t.Fatalf("expected ≥ 14 registered handlers, got %d", len(codes))
	}
	for _, kind := range codes {
		doc := handlers.GetHandlerDoc(kind)
		if strings.Contains(doc, "TODO") {
			t.Errorf("handler %s: doc contains TODO markers", kind)
		}
		for _, section := range requiredHandlerSections {
			if !strings.Contains(doc, section) {
				t.Errorf("handler %s: doc missing required section %q", kind, section)
			}
		}
	}
}
