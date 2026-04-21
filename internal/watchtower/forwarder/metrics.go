package forwarder

import (
	"encoding/json"
	"log/slog"
	"maps"
	"strings"

	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

// extractMetrics converts a sentinel MetricsPayload into VictoriaMetrics
// /api/v1/import lines.
//
// The sentinel collector emits raw gopsutil / docker JSON blobs under well-known
// keys in payload.Data; each key is mapped to a small fixed set of metrics.
// Gauges (instantaneous values): sentinel_host_cpu_percent,
// sentinel_host_memory_*_bytes, sentinel_host_disk_*_bytes,
// sentinel_container_memory_*_bytes. Counters (cumulative, safe for rate()):
// sentinel_container_cpu_usage_seconds_total,
// sentinel_{host,container}_network_{receive,transmit}_bytes_total.
//
// Every emitted line carries the validator label. Malformed or unknown keys
// are dropped; a Debug log is emitted for malformed JSON so that persistent
// shape drift is grep-able. Unchanged keys are already filtered upstream by
// the sentinel's delta layer.
func extractMetrics(validator string, payload protocol.MetricsPayload) []vmLine {
	ts := payload.CollectedAt.UnixMilli()
	lines := make([]vmLine, 0, 16)
	log := slog.Default()

	for key, raw := range payload.Data {
		switch key {
		case "cpu":
			lines = appendHostCPU(lines, validator, ts, raw, log)
		case "memory":
			lines = appendHostMemory(lines, validator, ts, raw, log)
		case "disk":
			lines = appendHostDisk(lines, validator, ts, raw, log)
		case "network":
			lines = appendHostNetwork(lines, validator, ts, raw, log)
		case "container":
			lines = appendContainer(lines, validator, ts, raw, log)
		case "config":
			lines = appendNodeConfig(lines, validator, ts, raw, log)
		case "self_stats":
			lines = appendSelfStats(lines, validator, ts, raw, log)
		}
	}
	return lines
}

// selfTypeStats mirrors internal/sentinel/self.TypeStats. Duplicated here
// because the watchtower must not import the sentinel's internal packages.
type selfTypeStats struct {
	UncompressedBytes int64            `json:"uncompressed_bytes"`
	WireBytes         int64            `json:"wire_bytes"`
	Drops             map[string]int64 `json:"drops"`
}

// appendSelfStats expands the sentinel's per-type counters into Prometheus
// counter series. The "encoding" label separates uncompressed (payload as
// JSON-marshaled) from wire (post-zstd for logs, identical for other types),
// letting the dashboard show compression ratio as `wire / uncompressed`.
// Drops carry a reason label ("buffer_full", "retry_exhausted", ...) so
// dashboards can distinguish transient backpressure from terminal failures.
func appendSelfStats(lines []vmLine, validator string, ts int64, raw json.RawMessage, log *slog.Logger) []vmLine {
	var byType map[string]selfTypeStats
	if err := json.Unmarshal(raw, &byType); err != nil {
		log.Debug("metrics: self_stats unmarshal failed", "validator", validator, "err", err)
		return lines
	}
	for typ, s := range byType {
		uncompLabels := map[string]string{"validator": validator, "type": typ, "encoding": "uncompressed"}
		wireLabels := map[string]string{"validator": validator, "type": typ, "encoding": "wire"}
		lines = append(lines,
			vmSample("sentinel_self_bytes_sent_total", uncompLabels, float64(s.UncompressedBytes), ts),
			vmSample("sentinel_self_bytes_sent_total", wireLabels, float64(s.WireBytes), ts),
		)
		for reason, n := range s.Drops {
			dropLabels := map[string]string{"validator": validator, "type": typ, "reason": reason}
			lines = append(lines, vmSample("sentinel_self_drops_total", dropLabels, float64(n), ts))
		}
	}
	return lines
}

// appendNodeConfig turns the metadata collector's {key: value} map into
// Prometheus info-style gauges: sentinel_node_config{validator, key, value}=1.
// The dashboard layer pivots this on key/validator so mismatches surface as
// rows with differing values across columns. Values stay as raw strings —
// the metadata collector already normalises them.
func appendNodeConfig(lines []vmLine, validator string, ts int64, raw json.RawMessage, log *slog.Logger) []vmLine {
	var values map[string]string
	if err := json.Unmarshal(raw, &values); err != nil {
		log.Debug("metrics: config unmarshal failed", "validator", validator, "err", err)
		return lines
	}
	for k, v := range values {
		lines = append(lines, vmSample("sentinel_node_config", map[string]string{
			"validator": validator,
			"key":       k,
			"value":     v,
		}, 1, ts))
	}
	return lines
}

// ---- Host extractors (gopsutil shapes)

func appendHostCPU(lines []vmLine, validator string, ts int64, raw json.RawMessage, log *slog.Logger) []vmLine {
	var percents []float64
	if err := json.Unmarshal(raw, &percents); err != nil {
		log.Debug("metrics: cpu unmarshal failed", "validator", validator, "err", err)
		return lines
	}
	if len(percents) == 0 {
		log.Debug("metrics: cpu shape drift (empty percents slice)", "validator", validator)
		return lines
	}
	return append(lines, vmLine{
		Metric:     map[string]string{"__name__": "sentinel_host_cpu_percent", "validator": validator},
		Values:     []float64{percents[0]},
		Timestamps: []int64{ts},
	})
}

type memoryStat struct {
	Total     uint64 `json:"total"`
	Available uint64 `json:"available"`
	Used      uint64 `json:"used"`
	Free      uint64 `json:"free"`
}

func appendHostMemory(lines []vmLine, validator string, ts int64, raw json.RawMessage, log *slog.Logger) []vmLine {
	var m memoryStat
	if err := json.Unmarshal(raw, &m); err != nil {
		log.Debug("metrics: memory unmarshal failed", "validator", validator, "err", err)
		return lines
	}
	base := map[string]string{"validator": validator}
	return append(lines,
		vmSample("sentinel_host_memory_total_bytes", base, float64(m.Total), ts),
		vmSample("sentinel_host_memory_available_bytes", base, float64(m.Available), ts),
		vmSample("sentinel_host_memory_used_bytes", base, float64(m.Used), ts),
		vmSample("sentinel_host_memory_free_bytes", base, float64(m.Free), ts),
	)
}

type diskStat struct {
	Path   string `json:"path"`
	Fstype string `json:"fstype"`
	Total  uint64 `json:"total"`
	Free   uint64 `json:"free"`
	Used   uint64 `json:"used"`
}

func appendHostDisk(lines []vmLine, validator string, ts int64, raw json.RawMessage, log *slog.Logger) []vmLine {
	var d diskStat
	if err := json.Unmarshal(raw, &d); err != nil {
		log.Debug("metrics: disk unmarshal failed", "validator", validator, "err", err)
		return lines
	}
	labels := map[string]string{"validator": validator, "path": d.Path, "fstype": d.Fstype}
	return append(lines,
		vmSample("sentinel_host_disk_total_bytes", labels, float64(d.Total), ts),
		vmSample("sentinel_host_disk_free_bytes", labels, float64(d.Free), ts),
		vmSample("sentinel_host_disk_used_bytes", labels, float64(d.Used), ts),
	)
}

type netStat struct {
	Name      string `json:"name"`
	BytesSent uint64 `json:"bytesSent"`
	BytesRecv uint64 `json:"bytesRecv"`
}

func appendHostNetwork(lines []vmLine, validator string, ts int64, raw json.RawMessage, log *slog.Logger) []vmLine {
	var nics []netStat
	if err := json.Unmarshal(raw, &nics); err != nil {
		log.Debug("metrics: network unmarshal failed", "validator", validator, "err", err)
		return lines
	}
	for _, n := range nics {
		labels := map[string]string{"validator": validator, "interface": n.Name}
		lines = append(lines,
			vmSample("sentinel_host_network_receive_bytes_total", labels, float64(n.BytesRecv), ts),
			vmSample("sentinel_host_network_transmit_bytes_total", labels, float64(n.BytesSent), ts),
		)
	}
	return lines
}

// ---- Container extractor (docker StatsResponse shape)

// containerStats is an intentionally anonymous projection of the subset of
// docker.types.container.StatsResponse we consume. Keeping it local avoids
// importing the docker SDK into watchtower (sentinel already owns that dep)
// and tolerates Docker API field drift as zero values rather than build errors.
type containerStats struct {
	Name     string `json:"name"`
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
	} `json:"cpu_stats"`
	MemoryStats struct {
		Usage uint64            `json:"usage"`
		Limit uint64            `json:"limit"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
}

func appendContainer(lines []vmLine, validator string, ts int64, raw json.RawMessage, log *slog.Logger) []vmLine {
	var c containerStats
	if err := json.Unmarshal(raw, &c); err != nil {
		log.Debug("metrics: container unmarshal failed", "validator", validator, "err", err)
		return lines
	}
	// docker's ContainerStatsOneShot response omits the Name field on some
	// API versions; fall back to the validator label so dashboards still have
	// a container identifier to group/legend by.
	name := strings.TrimPrefix(c.Name, "/")
	if name == "" {
		name = validator
	}
	labels := map[string]string{"validator": validator, "container": name}

	// Docker reports cumulative CPU time in nanoseconds; Prometheus convention is seconds.
	cpuSeconds := float64(c.CPUStats.CPUUsage.TotalUsage) / 1e9

	// Working set mirrors cadvisor's container_memory_working_set_bytes:
	// usage minus inactive_file. Falls back to usage when inactive_file is absent.
	workingSet := float64(c.MemoryStats.Usage)
	if inactive, ok := c.MemoryStats.Stats["inactive_file"]; ok && inactive <= c.MemoryStats.Usage {
		workingSet = float64(c.MemoryStats.Usage - inactive)
	}

	var rx, tx uint64
	for _, n := range c.Networks {
		rx += n.RxBytes
		tx += n.TxBytes
	}

	return append(lines,
		vmSample("sentinel_container_cpu_usage_seconds_total", labels, cpuSeconds, ts),
		vmSample("sentinel_container_memory_usage_bytes", labels, float64(c.MemoryStats.Usage), ts),
		vmSample("sentinel_container_memory_limit_bytes", labels, float64(c.MemoryStats.Limit), ts),
		vmSample("sentinel_container_memory_working_set_bytes", labels, workingSet, ts),
		vmSample("sentinel_container_network_receive_bytes_total", labels, float64(rx), ts),
		vmSample("sentinel_container_network_transmit_bytes_total", labels, float64(tx), ts),
	)
}

// ---- helpers

// vmSample builds a single-sample vmLine with the given name merged into labels.
// labels is cloned so callers can reuse their map across multiple samples
// without aliasing the __name__ entry.
func vmSample(name string, labels map[string]string, value float64, ts int64) vmLine {
	m := maps.Clone(labels)
	m["__name__"] = name
	return vmLine{
		Metric:     m,
		Values:     []float64{value},
		Timestamps: []int64{ts},
	}
}
