// cmd/beacon/main.go
//
// beacon is the sentry-side intermediary between a validator's sentinel and
// the central watchtower. It accepts Noise-authenticated connections from the
// sentinel, optionally augments /rpc payloads with the sentry's own view
// (peer count, p2p.pex, chain/version info), and forwards everything to the
// upstream watchtower over HTTPS.
//
// Subcommands:
//
//	run <config>              Start the beacon
//	generate-config <file>    Write an annotated example config to <file>
//	keygen <keys-dir>         Generate a Noise keypair (writes privkey+pubkey)
//	version [-v]              Print the build version
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/aeddi/gno-watchtower/internal/beacon/app"
	"github.com/aeddi/gno-watchtower/internal/beacon/config"
	pkglogger "github.com/aeddi/gno-watchtower/pkg/logger"
	"github.com/aeddi/gno-watchtower/pkg/noise"
	"github.com/aeddi/gno-watchtower/pkg/version"
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
		generateConfigCmd(os.Args[2:])
	case "keygen":
		keygenCmd(os.Args[2:])
	case "version":
		versionCmd(os.Args[2:])
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
		fmt.Fprintln(os.Stderr, "Usage: beacon run [--log-format=...] [--log-level=...] <config-file>")
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

func generateConfigCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: beacon generate-config <output-file>")
		os.Exit(1)
	}
	path := args[0]
	if err := config.Generate(path); err != nil {
		log.Fatalf("generate config: %v", err)
	}
	fmt.Printf("Config written to %s — open it to finish configuring your beacon.\n", path)
}

func keygenCmd(args []string) {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: beacon keygen <keys-dir>")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Generates a Noise static keypair for the beacon.")
		fmt.Fprintln(os.Stderr, "Writes <keys-dir>/privkey (mode 0600) and <keys-dir>/pubkey (mode 0644).")
		fmt.Fprintln(os.Stderr, "Prints the public key to stdout — share it with sentinel operators so they can pin it in [beacon] public_key.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}
	dir := fs.Arg(0)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Fatalf("create keys dir %s: %v", dir, err)
	}
	kp, err := noise.GenerateKeypair()
	if err != nil {
		log.Fatalf("generate keypair: %v", err)
	}
	if err := noise.WriteKeypair(dir, kp); err != nil {
		log.Fatalf("write keypair: %v", err)
	}
	fmt.Printf("%x\n", kp.Public)
	fmt.Fprintf(os.Stderr, "Wrote %s/privkey (mode 0600) and %s/pubkey (mode 0644).\n", dir, dir)
}

func versionCmd(args []string) {
	fs := flag.NewFlagSet("version", flag.ExitOnError)
	verbose := fs.Bool("v", false, "verbose: include commit, build time, Go toolchain")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if *verbose {
		fmt.Print(version.Long())
	} else {
		fmt.Println(version.Short())
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: beacon <command> [args]")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  run [--log-format=...] [--log-level=...] <config>  Start the beacon")
	fmt.Fprintln(os.Stderr, "  generate-config <output-file>                      Generate example config file")
	fmt.Fprintln(os.Stderr, "  keygen <keys-dir>                                  Generate Noise keypair")
	fmt.Fprintln(os.Stderr, "  version [-v]                                       Print the build version")
}
