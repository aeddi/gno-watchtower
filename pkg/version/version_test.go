package version

import (
	"strings"
	"testing"
)

func TestResolve_FallsBackToDevWhenNothingSet(t *testing.T) {
	saved := Version
	t.Cleanup(func() { Version = saved })
	Version = ""

	i := Resolve()
	// In tests, debug.ReadBuildInfo returns "(devel)" for the main module,
	// so the fallback ladder lands on "dev".
	if i.Version != "dev" {
		t.Errorf("Version: got %q, want %q", i.Version, "dev")
	}
	if i.GoVer == "" {
		t.Error("GoVer should never be empty")
	}
}

func TestResolve_UsesLdflagsValueWhenSet(t *testing.T) {
	saved := Version
	t.Cleanup(func() { Version = saved })
	Version = "v1.2.3"

	i := Resolve()
	if i.Version != "v1.2.3" {
		t.Errorf("Version: got %q, want %q", i.Version, "v1.2.3")
	}
}

func TestShort_MatchesResolveVersion(t *testing.T) {
	saved := Version
	t.Cleanup(func() { Version = saved })
	Version = "v0.9.0"

	if got := Short(); got != "v0.9.0" {
		t.Errorf("Short: got %q, want %q", got, "v0.9.0")
	}
}

func TestLong_ContainsVersionAndGo(t *testing.T) {
	saved := Version
	t.Cleanup(func() { Version = saved })
	Version = "v0.1.0"

	out := Long()
	if !strings.Contains(out, "version:  v0.1.0") {
		t.Errorf("Long output missing version line:\n%s", out)
	}
	if !strings.Contains(out, "go:") {
		t.Errorf("Long output missing go line:\n%s", out)
	}
}
