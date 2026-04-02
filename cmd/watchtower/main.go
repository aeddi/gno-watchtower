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

	"github.com/gnolang/val-companion/internal/watchtower/auth"
	"github.com/gnolang/val-companion/internal/watchtower/config"
	"github.com/gnolang/val-companion/internal/watchtower/forwarder"
	"github.com/gnolang/val-companion/internal/watchtower/handlers"
	"github.com/gnolang/val-companion/internal/watchtower/ratelimit"
	"github.com/gnolang/val-companion/internal/watchtower/stats"
	pkglogger "github.com/gnolang/val-companion/pkg/logger"
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
	fmt.Fprintf(os.Stderr, "Usage: watchtower <command> [args]\n\nCommands:\n  run [--log-format=...] [--log-level=...] <config>  Start the watchtower\n")
}
