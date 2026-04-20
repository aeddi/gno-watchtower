package metrics_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/watchtower/metrics"
	"github.com/aeddi/gno-watchtower/pkg/logger"
)

func TestMetrics_RecordAndScrape(t *testing.T) {
	m := metrics.New()
	m.RecordReceived("node-1", "rpc", 500)
	m.RecordReceived("node-1", "rpc", 200)
	m.RecordReceived("node-2", "logs", 1000)
	m.SetRetention(metrics.BackendLoki, 90*24*time.Hour, logger.Noop())
	m.SetRetention(metrics.BackendVM, 180*24*time.Hour, logger.Noop())

	srv := httptest.NewServer(m.Handler())
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	text := string(body)

	// Counter values.
	wants := []string{
		`watchtower_received_bytes_total{type="rpc",validator="node-1"} 700`,
		`watchtower_received_bytes_total{type="logs",validator="node-2"} 1000`,
		`watchtower_received_payloads_total{type="rpc",validator="node-1"} 2`,
		`watchtower_config_retention_seconds{backend="loki"} 7.776e+06`,
		`watchtower_config_retention_seconds{backend="vm"} 1.5552e+07`,
	}
	for _, w := range wants {
		if !strings.Contains(text, w) {
			t.Errorf("missing line in /metrics output:\n  want: %s\n\nactual output:\n%s", w, text)
		}
	}
}

func TestMetrics_ZeroRetention_EmitsWarnOnly(t *testing.T) {
	m := metrics.New()
	m.SetRetention(metrics.BackendLoki, 0, logger.Noop())

	srv := httptest.NewServer(m.Handler())
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `watchtower_config_retention_seconds{backend="loki"} 0`) {
		t.Errorf("expected 0 gauge for unconfigured retention, body: %s", string(body))
	}
}
