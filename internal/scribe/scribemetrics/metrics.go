package scribemetrics

import "github.com/prometheus/client_golang/prometheus"

type Registry struct {
	Registry *prometheus.Registry

	IngestObservations  *prometheus.CounterVec
	IngestDrops         *prometheus.CounterVec
	IngestBackoff       *prometheus.GaugeVec
	HandlerDuration     *prometheus.HistogramVec
	WriterQueueDepth    prometheus.Gauge
	WriterBatchDuration prometheus.Histogram
	WriterErrors        *prometheus.CounterVec
	EventsWritten       *prometheus.CounterVec
	SamplesWritten      prometheus.Counter
	AnchorsWritten      prometheus.Counter
	CompactDuration     prometheus.Histogram
	StorageBytes        *prometheus.GaugeVec
	StateCacheSubjects  prometheus.Gauge
	SSESubscribers      prometheus.Gauge
	SSEDrops            *prometheus.CounterVec
	APIRequests         *prometheus.CounterVec
	APIRequestDuration  *prometheus.HistogramVec
	BackfillJobs        *prometheus.GaugeVec
	BackfillChunks      *prometheus.CounterVec
}

func New() *Registry {
	r := prometheus.NewRegistry()
	m := &Registry{
		Registry: r,
		IngestObservations: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "scribe_ingest_observations_total"}, []string{"lane"}),
		IngestDrops: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "scribe_ingest_drops_total"}, []string{"lane", "reason"}),
		IngestBackoff: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{Name: "scribe_ingest_lane_backoff_seconds"}, []string{"lane"}),
		HandlerDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{Name: "scribe_normalizer_handler_duration_seconds"}, []string{"kind"}),
		WriterQueueDepth: prometheus.NewGauge(
			prometheus.GaugeOpts{Name: "scribe_writer_queue_depth"}),
		WriterBatchDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{Name: "scribe_writer_batch_duration_seconds"}),
		WriterErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "scribe_writer_errors_total"}, []string{"kind"}),
		EventsWritten: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "scribe_events_written_total"}, []string{"kind"}),
		SamplesWritten: prometheus.NewCounter(
			prometheus.CounterOpts{Name: "scribe_samples_written_total"}),
		AnchorsWritten: prometheus.NewCounter(
			prometheus.CounterOpts{Name: "scribe_anchors_written_total"}),
		CompactDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{Name: "scribe_compact_duration_seconds"}),
		StorageBytes: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{Name: "scribe_storage_bytes"}, []string{"table", "tier"}),
		StateCacheSubjects: prometheus.NewGauge(
			prometheus.GaugeOpts{Name: "scribe_state_cache_subjects"}),
		SSESubscribers: prometheus.NewGauge(
			prometheus.GaugeOpts{Name: "scribe_sse_subscribers"}),
		SSEDrops: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "scribe_sse_drops_total"}, []string{"reason"}),
		APIRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "scribe_api_requests_total"}, []string{"endpoint", "status"}),
		APIRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{Name: "scribe_api_request_duration_seconds"}, []string{"endpoint"}),
		BackfillJobs: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{Name: "scribe_backfill_jobs"}, []string{"status"}),
		BackfillChunks: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "scribe_backfill_chunks_total"}, []string{"result"}),
	}
	for _, c := range []prometheus.Collector{
		m.IngestObservations, m.IngestDrops, m.IngestBackoff,
		m.HandlerDuration, m.WriterQueueDepth, m.WriterBatchDuration,
		m.WriterErrors, m.EventsWritten, m.SamplesWritten, m.AnchorsWritten,
		m.CompactDuration, m.StorageBytes, m.StateCacheSubjects,
		m.SSESubscribers, m.SSEDrops, m.APIRequests, m.APIRequestDuration,
		m.BackfillJobs, m.BackfillChunks,
	} {
		r.MustRegister(c)
	}
	return m
}
