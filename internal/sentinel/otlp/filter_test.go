package otlp

import (
	"testing"

	collectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

// newReq builds a minimal ExportMetricsServiceRequest containing one
// ScopeMetrics with the given metric names.
func newReq(names ...string) *collectorpb.ExportMetricsServiceRequest {
	metrics := make([]*metricspb.Metric, 0, len(names))
	for _, n := range names {
		metrics = append(metrics, &metricspb.Metric{Name: n})
	}
	return &collectorpb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{
			{
				ScopeMetrics: []*metricspb.ScopeMetrics{
					{Metrics: metrics},
				},
			},
		},
	}
}

func metricNames(req *collectorpb.ExportMetricsServiceRequest) []string {
	var out []string
	for _, rm := range req.ResourceMetrics {
		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				out = append(out, m.Name)
			}
		}
	}
	return out
}

func TestFilterDenied_StripsDeniedNames(t *testing.T) {
	req := newReq(
		"block_interval_hist",
		"block_txs_hist",
		"inbound_peers_gauge",
		"validator_count_hist",
		"validator_vp_hist",
		"num_mempool_txs_hist",
		"num_cached_txs_hist",
		"http_request_time_hist",
	)
	filterDenied(req)
	want := []string{
		"block_interval_hist",
		"inbound_peers_gauge",
		"num_cached_txs_hist",
		"http_request_time_hist",
	}
	got := metricNames(req)
	if len(got) != len(want) {
		t.Fatalf("metric names: got %v, want %v", got, want)
	}
	wantSet := map[string]struct{}{}
	for _, n := range want {
		wantSet[n] = struct{}{}
	}
	for _, n := range got {
		if _, ok := wantSet[n]; !ok {
			t.Errorf("unexpected surviving metric %q", n)
		}
	}
}

func TestFilterDenied_EmptyRequest(t *testing.T) {
	req := newReq()
	filterDenied(req)
	if n := len(metricNames(req)); n != 0 {
		t.Errorf("expected no metrics, got %d", n)
	}
}

func TestFilterDenied_NilRequestSafe(t *testing.T) {
	// Should not panic.
	filterDenied(nil)
}

func TestFilterDenied_AllDeniedClearsScopeButKeepsShape(t *testing.T) {
	req := newReq("block_txs_hist", "validator_count_hist")
	filterDenied(req)
	// ResourceMetrics + ScopeMetrics survive even if Metrics is empty —
	// keeps downstream zero-alloc path simple.
	if len(req.ResourceMetrics) != 1 {
		t.Errorf("ResourceMetrics len = %d, want 1 (shape preserved)", len(req.ResourceMetrics))
	}
	if len(req.ResourceMetrics[0].ScopeMetrics) != 1 {
		t.Errorf("ScopeMetrics len = %d, want 1", len(req.ResourceMetrics[0].ScopeMetrics))
	}
	if n := len(req.ResourceMetrics[0].ScopeMetrics[0].Metrics); n != 0 {
		t.Errorf("Metrics len = %d, want 0", n)
	}
}

func TestDeniedMetricNames_ExpectedSet(t *testing.T) {
	// Guard against accidental additions/removals to the deny list without
	// a corresponding docs/rpc-vs-otlp.md update.
	want := map[string]struct{}{
		"block_txs_hist":       {},
		"validator_count_hist": {},
		"validator_vp_hist":    {},
		"num_mempool_txs_hist": {},
	}
	if len(deniedMetricNames) != len(want) {
		t.Errorf("deniedMetricNames size = %d, want %d", len(deniedMetricNames), len(want))
	}
	for k := range want {
		if _, ok := deniedMetricNames[k]; !ok {
			t.Errorf("deniedMetricNames missing %q", k)
		}
	}
}
