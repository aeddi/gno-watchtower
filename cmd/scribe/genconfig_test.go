package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateConfigWritesEmbeddedDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scribe.toml")
	var out bytes.Buffer
	if err := generateConfigCmd([]string{path}, &out); err != nil {
		t.Fatalf("generateConfigCmd: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(body), "[server]") {
		t.Errorf("default config missing [server] block")
	}
}

func TestGenerateConfigToStdout(t *testing.T) {
	var out bytes.Buffer
	if err := generateConfigCmd([]string{"-"}, &out); err != nil {
		t.Fatalf("generateConfigCmd: %v", err)
	}
	if !strings.Contains(out.String(), "[cluster]") {
		t.Errorf("stdout output missing [cluster]")
	}
}
