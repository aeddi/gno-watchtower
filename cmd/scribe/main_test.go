package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionShort(t *testing.T) {
	var out bytes.Buffer
	if err := versionCmd([]string{}, &out); err != nil {
		t.Fatalf("versionCmd: %v", err)
	}
	if !strings.Contains(out.String(), "scribe") {
		t.Errorf("expected 'scribe' in output, got %q", out.String())
	}
}

func TestUsageOnUnknownSubcommand(t *testing.T) {
	var out bytes.Buffer
	if err := dispatch([]string{"nope"}, &out); err == nil {
		t.Error("expected error for unknown subcommand")
	}
}
