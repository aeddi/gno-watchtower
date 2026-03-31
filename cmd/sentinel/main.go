package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "run":
		fmt.Fprintln(os.Stderr, "run: not yet implemented")
		os.Exit(1)
	case "generate-config":
		fmt.Fprintln(os.Stderr, "generate-config: not yet implemented")
		os.Exit(1)
	case "doctor":
		fmt.Fprintln(os.Stderr, "doctor: not yet implemented")
		os.Exit(1)
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: sentinel <command>\n\nCommands:\n  run              Start the sentinel\n  generate-config  Print example config to stdout\n  doctor           Check config and setup\n")
}
