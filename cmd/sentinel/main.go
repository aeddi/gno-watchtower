// cmd/sentinel/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gnolang/val-companion/internal/sentinel/app"
	"github.com/gnolang/val-companion/internal/sentinel/config"
	"github.com/gnolang/val-companion/internal/sentinel/doctor"
	pkglogger "github.com/gnolang/val-companion/pkg/logger"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "run":
		runCmd(os.Args[2:])
	case "generate-config":
		ctx := context.Background()
		if err := config.Generate(ctx, os.Stderr, os.Stdout); err != nil {
			log.Fatalf("generate config: %v", err)
		}
	case "doctor":
		doctorCmd(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	logFormat := fs.String("log-format", "console", "log output format: console, json, journal")
	logLevel := fs.String("log-level", "info", "minimum log level: debug, info, warn, error")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: sentinel run [--log-format=...] [--log-level=...] <config-file>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	cfg, err := config.Load(fs.Arg(0))
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	level, err := pkglogger.ParseLevel(*logLevel)
	if err != nil {
		log.Fatalf("invalid log level: %v", err)
	}
	logger, err := pkglogger.New(pkglogger.Format(*logFormat), level)
	if err != nil {
		log.Fatalf("init logger: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	app.Run(ctx, cfg, logger)
}

func doctorCmd(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: sentinel doctor <config-file>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	cfg, err := config.Load(fs.Arg(0))
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	code := doctor.Run(ctx, cfg, fs.Arg(0), os.Stdout)
	os.Exit(code)
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: sentinel <command> [args]\n\nCommands:\n  run [--log-format=...] [--log-level=...] <config>  Start the sentinel\n  generate-config                                    Print example config to stdout\n  doctor <config>                                    Check config and setup\n")
}
