package handlers

import (
	"context"
	"strings"
	"testing"

	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

func TestRegisterHandlerStoresFactory(t *testing.T) {
	resetHandlerRegistryForTest(t)
	doc := "# fake.kind\n## What it represents\nA fake.\n"
	Register("fake.kind", func(cluster string) Handler { return fakeHandler{} }, doc)

	codes := RegisteredKinds()
	if len(codes) != 1 || codes[0] != "fake.kind" {
		t.Fatalf("RegisteredKinds = %v", codes)
	}
	if got := GetHandlerDoc("fake.kind"); !strings.Contains(got, "## What it represents") {
		t.Errorf("GetHandlerDoc = %q", got)
	}
	if got := NewHandler("fake.kind", "c1"); got == nil {
		t.Errorf("NewHandler returned nil")
	}
}

func TestRegisterRejectsEmptyDoc(t *testing.T) {
	resetHandlerRegistryForTest(t)
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on empty doc, got nil")
		}
	}()
	Register("fake.kind", func(string) Handler { return fakeHandler{} }, "")
}

func TestRegisterRejectsDuplicate(t *testing.T) {
	resetHandlerRegistryForTest(t)
	Register("fake.kind", func(string) Handler { return fakeHandler{} }, "doc")
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on duplicate Register, got nil")
		}
	}()
	Register("fake.kind", func(string) Handler { return fakeHandler{} }, "doc")
}

type fakeHandler struct{}

func (fakeHandler) Name() string { return "fake" }
func (fakeHandler) Meta() Meta {
	return Meta{Kind: "fake.kind", Source: SourceLog, Description: "fake", DocRef: "/docs/handlers/fake.kind"}
}

func (fakeHandler) Handle(_ context.Context, _ normalizer.Observation) []types.Op {
	return nil
}
