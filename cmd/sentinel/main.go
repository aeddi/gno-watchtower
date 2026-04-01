// cmd/sentinel/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gnolang/val-companion/internal/sentinel/app"
	"github.com/gnolang/val-companion/internal/sentinel/config"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "run":
		runCmd()
	case "generate-config":
		fmt.Print(config.Example)
	case "doctor":
		fmt.Fprintln(os.Stderr, "doctor: not yet implemented")
		os.Exit(1)
	default:
		usage()
		os.Exit(1)
	}
}

func runCmd() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: sentinel run <config-file>")
		os.Exit(1)
	}
	cfg, err := config.Load(os.Args[2])
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app.Run(ctx, cfg)
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: sentinel <command> [args]\n\nCommands:\n  run <config>     Start the sentinel\n  generate-config  Print example config to stdout\n  doctor <config>  Check config and setup\n")
}
