package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/analysis"
	_ "github.com/aeddi/gno-watchtower/internal/scribe/analysis/rules" // load registry for doctor reports
	"github.com/aeddi/gno-watchtower/internal/scribe/config"
	"github.com/aeddi/gno-watchtower/internal/scribe/store"
)

// doctorCmd validates the config, pings sources, and opens the DuckDB store.
// Writes a one-line-per-check report to `out`. Returns a non-nil error if any
// check failed.
func doctorCmd(args []string, out io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: scribe doctor <config.toml>")
	}
	cfg, err := config.Load(args[0])
	if err != nil {
		fmt.Fprintf(out, "FAIL config: %v\n", err)
		return err
	}
	fmt.Fprintf(out, "OK   config: cluster.id=%q, listen=%s\n", cfg.Cluster.ID, cfg.Server.ListenAddr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var bad []string

	// VM ping: GET /api/v1/query?query=up
	if err := pingHTTP(ctx, cfg.Sources.VM.URL+"/api/v1/query?query=up"); err != nil {
		fmt.Fprintf(out, "FAIL victoria_metrics: %v\n", err)
		bad = append(bad, "victoria_metrics")
	} else {
		fmt.Fprintf(out, "OK   victoria_metrics: %s reachable\n", cfg.Sources.VM.URL)
	}

	// Loki ping: GET /ready, fallback to /loki/api/v1/labels.
	if err := pingHTTP(ctx, cfg.Sources.Loki.URL+"/ready"); err != nil {
		if err2 := pingHTTP(ctx, cfg.Sources.Loki.URL+"/loki/api/v1/labels"); err2 != nil {
			fmt.Fprintf(out, "FAIL loki: %v\n", err)
			bad = append(bad, "loki")
		} else {
			fmt.Fprintf(out, "OK   loki: %s reachable (/loki/api/v1/labels)\n", cfg.Sources.Loki.URL)
		}
	} else {
		fmt.Fprintf(out, "OK   loki: %s reachable (/ready)\n", cfg.Sources.Loki.URL)
	}

	// Store: open + report schema version.
	dbPath := cfg.Storage.DBPath
	if dbPath != "" && filepath.Dir(dbPath) != "." {
		_ = os.MkdirAll(filepath.Dir(dbPath), 0o755)
	}
	s, err := store.Open(dbPath)
	if err != nil {
		// DuckDB single-writer lock: scribe is already running against this DB.
		// That's informational, not a failure — doctor in a live cluster is the
		// expected case.
		if strings.Contains(err.Error(), "Conflicting lock") {
			fmt.Fprintf(out, "OK   store: %s in use by running scribe (lock held)\n", dbPath)
		} else {
			fmt.Fprintf(out, "FAIL store: %v\n", err)
			bad = append(bad, "store")
		}
	} else {
		v, _ := s.SchemaVersion(ctx)
		fmt.Fprintf(out, "OK   store: %s schema_version=%d\n", dbPath, v)
		_ = s.Close()
	}

	codes := analysis.RegisteredCodes()
	fmt.Fprintf(out, "OK   analysis: %d rules registered\n", len(codes))
	for _, k := range codes {
		m := analysis.GetMeta(k)
		doc := analysis.GetDoc(k)
		if doc == "" {
			fmt.Fprintf(out, "FAIL analysis: rule %s has empty doc\n", k)
			bad = append(bad, "analysis:"+k)
			continue
		}
		fmt.Fprintf(out, "       %s severity=%s tick=%s kinds=%v\n", k, m.Severity, m.TickPeriod, m.Kinds)
	}

	if len(bad) > 0 {
		return fmt.Errorf("doctor failures: %v", bad)
	}
	return nil
}

func pingHTTP(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	return nil
}
