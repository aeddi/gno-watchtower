// Package version reports the build version and VCS metadata for the
// sentinel and watchtower binaries.
//
// Version is set at build time by the release pipeline via:
//
//	go build -ldflags "-X github.com/aeddi/gno-watchtower/pkg/version.Version=$VERSION"
//
// When Version is empty (e.g. a plain `go install` or `go build` without
// -ldflags), Resolve falls back to Go's built-in module + VCS metadata via
// runtime/debug.ReadBuildInfo, so `sentinel version` still reports a real
// version for users who install via `go install …@v1.2.3`.
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// Version is overridden at release-pipeline build time. The empty default lets
// the BuildInfo fallback take over for go-install builds.
var Version = ""

// Info holds resolved version + VCS metadata.
type Info struct {
	Version string // semver, pseudo-version, or "dev"
	Commit  string // vcs.revision from Go's build info (may be empty)
	Built   string // vcs.time RFC3339 from Go's build info (may be empty)
	GoVer   string // runtime.Version(), e.g. "go1.25.0"
}

// Resolve returns the version info. Prefers the ldflags-injected Version;
// falls back to runtime/debug.ReadBuildInfo for go-install builds; ultimately
// falls back to "dev" when no version signal is available.
func Resolve() Info {
	info := Info{Version: Version, GoVer: runtime.Version()}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		if info.Version == "" {
			info.Version = "dev"
		}
		return info
	}
	if info.Version == "" {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			info.Version = v
		} else {
			info.Version = "dev"
		}
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			info.Commit = s.Value
		case "vcs.time":
			info.Built = s.Value
		}
	}
	return info
}

// Short returns just the version string for `sentinel version`.
func Short() string { return Resolve().Version }

// Long returns a multi-line verbose summary for `sentinel version -v`.
// Commit and Built lines are omitted when their underlying fields are empty
// (e.g. when the binary was built outside a git checkout).
func Long() string {
	i := Resolve()
	out := fmt.Sprintf("  version:  %s\n", i.Version)
	if i.Commit != "" {
		out += fmt.Sprintf("  commit:   %s\n", i.Commit)
	}
	if i.Built != "" {
		out += fmt.Sprintf("  built:    %s\n", i.Built)
	}
	out += fmt.Sprintf("  go:       %s\n", i.GoVer)
	return out
}
