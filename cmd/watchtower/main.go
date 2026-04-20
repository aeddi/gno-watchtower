// cmd/watchtower/main.go
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aeddi/gno-watchtower/internal/watchtower/auth"
	"github.com/aeddi/gno-watchtower/internal/watchtower/config"
	"github.com/aeddi/gno-watchtower/internal/watchtower/forwarder"
	"github.com/aeddi/gno-watchtower/internal/watchtower/handlers"
	"github.com/aeddi/gno-watchtower/internal/watchtower/ratelimit"
	"github.com/aeddi/gno-watchtower/internal/watchtower/stats"
	pkglogger "github.com/aeddi/gno-watchtower/pkg/logger"
)

const statsInterval = time.Hour

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
	default:
		usage()
		os.Exit(1)
	}
}

func generateConfigCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: watchtower generate-config <output-file>")
		os.Exit(1)
	}
	path := args[0]

	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("create %s: %v", path, err)
	}

	if err := config.Generate(f); err != nil {
		f.Close()
		if rmErr := os.Remove(path); rmErr != nil {
			log.Printf("warning: failed to clean up %s: %v", path, rmErr)
		}
		log.Fatalf("generate config: %v", err)
	}
	if err := f.Close(); err != nil {
		log.Fatalf("close %s: %v", path, err)
	}

	fmt.Printf("Config written to %s — open it to finish configuring your watchtower.\n", path)
}

func runCmd(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	logFormat := fs.String("log-format", "console", "log output format: console, json, journal")
	logLevel := fs.String("log-level", "info", "minimum log level: debug, info, warn, error")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: watchtower run [--log-format=...] [--log-level=...] <config-file>")
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

	a := auth.New(cfg.Validators, cfg.Security.BanThreshold, cfg.Security.BanDuration.Duration)

	// SIGHUP: reload config and update auth tokens.
	sighupCh := make(chan os.Signal, 1)
	signal.Notify(sighupCh, syscall.SIGHUP)
	defer signal.Stop(sighupCh)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-sighupCh:
				newCfg, err := config.Load(fs.Arg(0))
				if err != nil {
					logger.Error("reload config", "err", err)
					continue
				}
				a.Reload(newCfg.Validators)
				logger.Info("config reloaded", "validators", len(newCfg.Validators))
			}
		}
	}()

	rl := ratelimit.New(cfg.Security.RateLimitRPS, cfg.Security.RateLimitBurst)
	fwd := forwarder.New(cfg.VictoriaMetrics.URL, cfg.Loki.URL)
	st := stats.New()
	srv := handlers.NewServer(cfg, a, rl, fwd, st, logger)

	statsTicker := time.NewTicker(statsInterval)
	defer statsTicker.Stop()
	go srv.RunStatsLogger(ctx, statsTicker)

	httpSrv := &http.Server{
		Addr:    cfg.Server.ListenAddr,
		Handler: srv.Handler(),
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutCtx); err != nil {
			logger.Error("shutdown", "err", err)
		}
	}()

	logger.Info("watchtower starting", "addr", cfg.Server.ListenAddr)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: watchtower <command> [args]")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  run [--log-format=...] [--log-level=...] <config>  Start the watchtower")
	fmt.Fprintln(os.Stderr, "  generate-config <output-file>                      Generate example config file")
}
