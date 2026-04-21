// cmd/watchtower/main.go
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/aeddi/gno-watchtower/internal/watchtower/auth"
	"github.com/aeddi/gno-watchtower/internal/watchtower/config"
	"github.com/aeddi/gno-watchtower/internal/watchtower/forwarder"
	"github.com/aeddi/gno-watchtower/internal/watchtower/handlers"
	wtmetrics "github.com/aeddi/gno-watchtower/internal/watchtower/metrics"
	"github.com/aeddi/gno-watchtower/internal/watchtower/ratelimit"
	"github.com/aeddi/gno-watchtower/internal/watchtower/stats"
	pkglogger "github.com/aeddi/gno-watchtower/pkg/logger"
	"github.com/aeddi/gno-watchtower/pkg/version"
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
	case "version":
		versionCmd(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
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

	m := wtmetrics.New()
	m.SetRetention(wtmetrics.BackendLoki, parseLokiRetention(os.Getenv("LOGS_RETENTION"), logger), logger)
	m.SetRetention(wtmetrics.BackendVM, parseVMRetention(os.Getenv("METRICS_RETENTION"), logger), logger)

	rl := ratelimit.New(cfg.Security.RateLimitRPS, cfg.Security.RateLimitBurst, m.RecordRateLimited)
	fwd := forwarder.New(cfg.VictoriaMetrics.URL, cfg.Loki.URL, m.RecordLogsBelowMinLevel)
	st := stats.New()

	srv := handlers.NewServer(cfg, a, rl, fwd, st, m, logger)

	statsTicker := time.NewTicker(statsInterval)
	defer statsTicker.Stop()
	go srv.RunStatsLogger(ctx, statsTicker)

	httpSrv := &http.Server{
		Addr:    cfg.Server.ListenAddr,
		Handler: srv.Handler(),
		// Slowloris / hung-connection defenses. ReadTimeout is generous to
		// cover worst-case 50 MiB batch uploads on a throttled link
		// (~420 KB/s × 120s). Bodies larger than 50 MiB are rejected via
		// http.MaxBytesReader in the handler, so 120s is a comfortable ceiling.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
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

// parseLokiRetention converts the Loki retention_period format (Go duration
// like "2160h") into a time.Duration. Empty input returns 0 which the
// retention gauge reports unset.
func parseLokiRetention(s string, log *slog.Logger) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Warn("parse LOGS_RETENTION failed — gauge will report 0", "raw", s, "err", err)
		return 0
	}
	return d
}

// parseVMRetention converts VictoriaMetrics' -retentionPeriod flag format
// (bare integer = months, or e.g. "1y", "30d", "720h") into a time.Duration.
// We don't link VM's parser — it'd pull the whole VM module — so we handle the
// common suffixes with a month = 30d approximation good enough for dashboards.
func parseVMRetention(s string, log *slog.Logger) time.Duration {
	if s == "" {
		return 0
	}
	// Bare integer → months.
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return time.Duration(n) * 30 * 24 * time.Hour
	}
	// Suffixed: VM accepts d/w/y in addition to Go's h/m/s. Translate to hours
	// before handing off to time.ParseDuration.
	switch last := s[len(s)-1]; last {
	case 'd', 'w', 'y':
		n, err := strconv.Atoi(s[:len(s)-1])
		if err != nil || n <= 0 {
			log.Warn("parse METRICS_RETENTION failed — gauge will report 0", "raw", s, "err", err)
			return 0
		}
		mult := map[byte]time.Duration{
			'd': 24 * time.Hour,
			'w': 7 * 24 * time.Hour,
			'y': 365 * 24 * time.Hour,
		}[last]
		return time.Duration(n) * mult
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	log.Warn("parse METRICS_RETENTION failed — gauge will report 0", "raw", s)
	return 0
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: watchtower <command> [args]")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  run [--log-format=...] [--log-level=...] <config>  Start the watchtower")
	fmt.Fprintln(os.Stderr, "  generate-config <output-file>                      Generate example config file")
	fmt.Fprintln(os.Stderr, "  version [-v]                                       Print the build version")
}
