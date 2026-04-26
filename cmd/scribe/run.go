package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/anchor"
	"github.com/aeddi/gno-watchtower/internal/scribe/api"
	"github.com/aeddi/gno-watchtower/internal/scribe/backfill"
	"github.com/aeddi/gno-watchtower/internal/scribe/cache"
	"github.com/aeddi/gno-watchtower/internal/scribe/compactor"
	"github.com/aeddi/gno-watchtower/internal/scribe/config"
	"github.com/aeddi/gno-watchtower/internal/scribe/handlers"
	"github.com/aeddi/gno-watchtower/internal/scribe/ingest"
	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/scribemetrics"
	"github.com/aeddi/gno-watchtower/internal/scribe/sources/loki"
	"github.com/aeddi/gno-watchtower/internal/scribe/sources/vm"
	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
	"github.com/aeddi/gno-watchtower/internal/scribe/writer"
)

// runCmd is the public CLI entrypoint dispatched by main().
func runCmd(args []string, _ io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: scribe run <config.toml>")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return runCmdImpl(ctx, args[0], nil)
}

// runCmdImpl is the testable form. addrCh, if non-nil, receives the bound
// listen address as soon as the listener is up.
func runCmdImpl(ctx context.Context, configPath string, addrCh chan<- string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	configureLogging(cfg.Logging)

	s, err := store.Open(cfg.Storage.DBPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	metrics := scribemetrics.New()
	c := cache.New()
	w := writer.New(s, writer.Config{
		BatchSize:   cfg.Writer.BatchSize,
		BatchWindow: cfg.Writer.BatchWindow.Std(),
		Metrics:     metrics,
	})

	hs := []normalizer.Handler{
		// Metric-driven handlers
		handlers.NewHeight(cfg.Cluster.ID),
		handlers.NewOnline(cfg.Cluster.ID),
		handlers.NewPeers(cfg.Cluster.ID),
		handlers.NewMempool(cfg.Cluster.ID),
		handlers.NewVotingPower(cfg.Cluster.ID),
		handlers.NewValsetSize(cfg.Cluster.ID),
		// Log-driven handlers
		handlers.NewProposed(cfg.Cluster.ID),
		handlers.NewConsensusRoundStep(cfg.Cluster.ID),
		handlers.NewVoteCast(cfg.Cluster.ID),
		handlers.NewPeerConnected(cfg.Cluster.ID),
		handlers.NewPeerDisconnected(cfg.Cluster.ID),
		handlers.NewBlockCommitted(cfg.Cluster.ID),
		handlers.NewValsetChanged(cfg.Cluster.ID),
		handlers.NewTxExecuted(cfg.Cluster.ID),
	}
	opCh := make(chan types.Op, 1024)
	n := normalizer.New(opCh, hs)

	go func() {
		for op := range opCh {
			w.Submit(op)
		}
	}()
	go w.Run(ctx)
	go n.Run(ctx)

	vmCli := vm.New(cfg.Sources.VM.URL)
	lokiCli := loki.New(cfg.Sources.Loki.URL)

	fastLane := ingest.NewFastLane(vmCli, cfg.Ingest.Fast.Queries, cfg.Ingest.Fast.Interval.Std(), n.Input(normalizer.LaneFast)).WithMetrics(metrics)
	slowLane := ingest.NewSlowLane(vmCli, cfg.Ingest.Slow.Queries, cfg.Ingest.Slow.Interval.Std(), n.Input(normalizer.LaneSlow)).WithMetrics(metrics)
	logsLane := ingest.NewLogsLane(cfg.Sources.Loki.URL, cfg.Ingest.Logs.Streams, cfg.Ingest.Logs.OverlapWindow.Std(), n.Input(normalizer.LaneLogs)).WithMetrics(metrics)
	go fastLane.Run(ctx)
	go slowLane.Run(ctx)
	go logsLane.Run(ctx)

	// Periodic storage-bytes gauge updater. Reads row counts from the store every
	// 30 s and exposes them as scribe_storage_bytes{table,tier}. Tier label is
	// fixed to "0" for now since StorageBytes returns row counts not on-disk
	// bytes (the metric name is misleading; we'll rename when we add real bytes).
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				rows, err := s.StorageBytes(ctx)
				if err != nil {
					continue
				}
				for table, n := range rows {
					metrics.StorageBytes.WithLabelValues(table, "0").Set(float64(n))
				}
			}
		}
	}()

	a := anchor.New(c, w, cfg.Cluster.ID)
	go a.Run(ctx, cfg.Anchors.Interval.Std())

	cmp := compactor.New(s, cfg.Cluster.ID, compactor.Config{
		HotWindow:  cfg.Retention.HotWindow.Std(),
		WarmBucket: cfg.Retention.WarmBucket.Std(),
		PruneAfter: cfg.Retention.PruneAfter.Std(),
		CompactAt:  cfg.Retention.CompactAt,
		Jitter:     cfg.Retention.CompactJitter.Std(),
	})
	go cmp.Run(ctx)

	bfEngine := backfill.New(backfill.Deps{
		Store:       s,
		ClusterID:   cfg.Cluster.ID,
		VM:          vmCli,
		Loki:        lokiCli,
		FastQueries: cfg.Ingest.Fast.Queries,
		SlowQueries: cfg.Ingest.Slow.Queries,
		LogStreams:  cfg.Ingest.Logs.Streams,
		ChunkSize:   cfg.Backfill.ChunkSize.Std(),
		Normalizer:  n,
	})
	bfSched := backfill.NewScheduler(s, bfEngine, cfg.Cluster.ID, backfill.SchedulerConfig{
		PollInterval:     5 * time.Second,
		ResumeStaleAfter: cfg.Backfill.ResumeStaleAfter.Std(),
	})
	go bfSched.Run(ctx)

	apiSrv := api.New(api.Deps{Store: s, Cache: c, Writer: w, ClusterID: cfg.Cluster.ID, Metrics: metrics})

	ln, err := net.Listen("tcp", cfg.Server.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	if addrCh != nil {
		addrCh <- ln.Addr().String()
	}
	hsrv := &http.Server{
		Handler:           apiSrv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = hsrv.Shutdown(shutdownCtx)
	}()
	if err := hsrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func configureLogging(cfg config.Logging) {
	level := slog.LevelInfo
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	var h slog.Handler
	switch cfg.Format {
	case "console":
		h = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	default:
		h = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	}
	slog.SetDefault(slog.New(h))
}
