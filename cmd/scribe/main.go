// cmd/scribe/main.go
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/aeddi/gno-watchtower/pkg/version"
)

func main() {
	if err := dispatch(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func dispatch(args []string, out io.Writer) error {
	if len(args) == 0 {
		return usage(out)
	}
	switch args[0] {
	case "version":
		return versionCmd(args[1:], out)
	case "run":
		return runCmd(args[1:], out)
	case "generate-config":
		return generateConfigCmd(args[1:], out)
	case "doctor":
		return doctorCmd(args[1:], out)
	case "backfill":
		return backfillCmd(args[1:], out)
	default:
		return usage(out)
	}
}

func usage(out io.Writer) error {
	fmt.Fprintln(out, "usage: scribe <subcommand> [args]")
	fmt.Fprintln(out, "subcommands: run, doctor, generate-config, backfill, version")
	return errors.New("unknown subcommand")
}

func versionCmd(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(out)
	verbose := fs.Bool("v", false, "verbose: include commit, build time, Go toolchain")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *verbose {
		fmt.Fprint(out, "scribe "+version.Long())
	} else {
		fmt.Fprintln(out, "scribe "+version.Short())
	}
	return nil
}
