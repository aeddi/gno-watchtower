// Package metrics registers and exposes Prometheus counters for traffic the
// watchtower receives, plus gauges for operational config (retention limits).
// The /metrics endpoint is scraped by VictoriaMetrics from inside the Docker
// network — there is no auth layer because the endpoint is not reachable from
// outside the compose stack.
package metrics

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Backend identifies a storage backend for retention gauges.
type Backend string

const (
	BackendLoki Backend = "loki"
	BackendVM   Backend = "vm"
)

// Metrics holds the Prometheus collectors exposed by the watchtower's /metrics
// endpoint. Kept on a struct so tests can build an isolated registry rather
// than mutate the default one (parallel tests otherwise collide on global
// register/unregister).
type Metrics struct {
	registry          *prometheus.Registry
	receivedBytes     *prometheus.CounterVec
	receivedPayloads  *prometheus.CounterVec
	rateLimited       *prometheus.CounterVec
	logsBelowMinLevel *prometheus.CounterVec
	authFailures      *prometheus.CounterVec
	permissionDenied  *prometheus.CounterVec
	retention         *prometheus.GaugeVec
}

// New builds a fresh Metrics with all collectors registered on a private
// registry. Use Handler() to expose them.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		registry: reg,
		receivedBytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "watchtower_received_bytes_total",
			Help: "Total bytes received from sentinels, broken down by validator and data type.",
		}, []string{"validator", "type"}),
		receivedPayloads: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "watchtower_received_payloads_total",
			Help: "Total payloads received from sentinels, broken down by validator and data type.",
		}, []string{"validator", "type"}),
		rateLimited: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "watchtower_rate_limited_total",
			Help: "Requests rejected with HTTP 429 by the per-validator rate limiter.",
		}, []string{"validator"}),
		logsBelowMinLevel: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "watchtower_logs_below_min_level_total",
			Help: "Log payloads dropped because their level was below the validator's configured logs_min_level.",
		}, []string{"validator"}),
		authFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "watchtower_auth_failures_total",
			Help: "Auth-middleware rejections broken down by reason (invalid_token, banned). Not keyed on validator because rejected requests carry no trusted validator identity.",
		}, []string{"reason"}),
		permissionDenied: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "watchtower_permission_denied_total",
			Help: "Per-validator 403 counts, labelled by the permission that was missing. Useful to spot sentinels sending data the operator never authorised them for.",
		}, []string{"validator", "permission"}),
		retention: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "watchtower_config_retention_seconds",
			Help: "Configured retention window per storage backend, in seconds.",
		}, []string{"backend"}),
	}
	reg.MustRegister(m.receivedBytes, m.receivedPayloads, m.rateLimited, m.logsBelowMinLevel, m.authFailures, m.permissionDenied, m.retention)
	// Register Go + process metrics so we also get go_goroutines, process_cpu_seconds_total, etc.
	reg.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)
	return m
}

// RecordReceived increments the byte + payload counters for the given
// authenticated validator and data type.
func (m *Metrics) RecordReceived(validator, dataType string, bytes int) {
	m.receivedBytes.WithLabelValues(validator, dataType).Add(float64(bytes))
	m.receivedPayloads.WithLabelValues(validator, dataType).Inc()
}

// RecordRateLimited bumps the counter for a validator whose request was
// rejected with HTTP 429 by the per-validator rate limiter. Called from the
// rate-limit middleware; cheap enough to call in the hot path.
func (m *Metrics) RecordRateLimited(validator string) {
	m.rateLimited.WithLabelValues(validator).Inc()
}

// RecordLogsBelowMinLevel bumps the counter for a validator whose log
// payload was dropped by the server-side logs_min_level filter. This is an
// intentional filter, not an error — the counter exists so operators can
// see at a glance that an aggressive min_level is eating traffic.
func (m *Metrics) RecordLogsBelowMinLevel(validator string) {
	m.logsBelowMinLevel.WithLabelValues(validator).Inc()
}

// RecordAuthFailure bumps the per-reason counter for an auth-middleware
// rejection. Reason is a short slug (e.g. "invalid_token", "banned"). No
// validator label — rejected requests carry no trusted validator identity.
func (m *Metrics) RecordAuthFailure(reason string) {
	m.authFailures.WithLabelValues(reason).Inc()
}

// RecordPermissionDenied bumps the counter for a 403 that was issued because
// the authenticated validator lacks the permission required by the endpoint.
// Unlike auth failures, the validator IS known here (auth already passed).
func (m *Metrics) RecordPermissionDenied(validator, permission string) {
	m.permissionDenied.WithLabelValues(validator, permission).Inc()
}

// SetRetention publishes the retention window for a backend as a gauge.
// Called at startup from parsed config. A zero duration leaves the gauge unset
// rather than at 0, so dashboards can tell "not configured" from "zero retention".
func (m *Metrics) SetRetention(backend Backend, d time.Duration, log *slog.Logger) {
	if d <= 0 {
		log.Warn("retention not set — gauge will report 0", "backend", backend)
	}
	m.retention.WithLabelValues(string(backend)).Set(d.Seconds())
}

// SetBannedCountSource registers a GaugeFunc that reports the number of IPs
// currently under an active ban. The source closure is evaluated on every
// Prometheus scrape — callers should hand off a cheap, lock-friendly accessor
// (e.g. Authenticator.BannedCount). Intentionally a setter rather than a New()
// param so wire-up stays optional: tests and standalone runs can skip it and
// the gauge simply won't appear in /metrics.
func (m *Metrics) SetBannedCountSource(src func() int) {
	m.registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "watchtower_banned_ips",
		Help: "Count of IPs currently under an active ban. Sampled at scrape time from the auth layer's live ban map.",
	}, func() float64 { return float64(src()) }))
}

// Handler returns the http.Handler for Prometheus scrapes.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
