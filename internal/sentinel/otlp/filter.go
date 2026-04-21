package otlp

import (
	collectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
)

// deniedMetricNames lists OTLP metrics that gnoland emits but the sentinel
// drops before forwarding, because the sentinel's RPC collector provides a
// fresher, more actionable equivalent (3s poll vs 60s OTLP export). Keeping
// the filter at the sentinel saves bandwidth on the sentinel→watchtower link
// and prevents the two pipelines from fighting for the same dashboard slot.
//
// See docs/data-collected.md for the rationale behind each entry.
// Package-private and never mutated after init — safe for concurrent reads
// from the HTTP handler goroutines.
var deniedMetricNames = map[string]struct{}{
	"block_txs_hist":       {}, // superseded by sentinel_rpc_block_num_txs
	"validator_count_hist": {}, // superseded by sentinel_rpc_validator_set_size
	"validator_vp_hist":    {}, // superseded by sentinel_rpc_validator_set_total_power
	"num_mempool_txs_hist": {}, // superseded by sentinel_rpc_mempool_txs (always-emit)
}

// filterDenied strips every Metric whose name is in deniedMetricNames from req
// in place. In-place mutation is safe because proto.Unmarshal produces a fresh
// ExportMetricsServiceRequest per HTTP call, so the sentinel is the sole
// owner of this message for the duration of handleMetrics. ResourceMetrics and
// ScopeMetrics entries whose Metrics slice goes empty are left in the request —
// the OTLP exporter tolerates them and removal would cost allocations for no gain.
func filterDenied(req *collectorpb.ExportMetricsServiceRequest) {
	if req == nil {
		return
	}
	for _, rm := range req.ResourceMetrics {
		for _, sm := range rm.ScopeMetrics {
			out := sm.Metrics[:0]
			for _, m := range sm.Metrics {
				if _, denied := deniedMetricNames[m.Name]; denied {
					continue
				}
				out = append(out, m)
			}
			sm.Metrics = out
		}
	}
}
