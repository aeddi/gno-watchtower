// cmd/scribe/main.go
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/aeddi/gno-watchtower/pkg/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "version":
		versionCmd(os.Args[2:], os.Stdout)
	default:
		usage()
		os.Exit(1)
	}
}

func versionCmd(args []string, out io.Writer) {
	fs := flag.NewFlagSet("version", flag.ExitOnError)
	verbose := fs.Bool("v", false, "verbose: include commit, build time, Go toolchain")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if *verbose {
		fmt.Fprint(out, version.Long())
	} else {
		fmt.Fprintln(out, version.Short())
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: scribe <command> [args]")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  version [-v]  Print the build version")
}

// captureVersionOutput runs versionCmd with args and returns the output as a string.
// Used only by tests.
func captureVersionOutput(args []string) string {
	var buf bytes.Buffer
	versionCmd(args, &buf)
	return buf.String()
}
