package main

import (
	"strings"
	"testing"
)

func TestVersionCmd_Short(t *testing.T) {
	t.Parallel()
	out := captureVersionOutput([]string{})
	if out == "" {
		t.Error("version cmd produced no output")
	}
}

func TestVersionCmd_Verbose(t *testing.T) {
	t.Parallel()
	out := captureVersionOutput([]string{"-v"})
	if !strings.Contains(out, "version:") {
		t.Errorf("verbose output missing version: line:\n%s", out)
	}
	if !strings.Contains(out, "go:") {
		t.Errorf("verbose output missing go: line:\n%s", out)
	}
}
