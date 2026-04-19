package forwarder

import (
	"encoding/json"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/pkg/protocol"
)

// ---- Fixtures matching the real gopsutil / docker JSON shapes the sentinel collector emits.

const (
	hostCPUJSON     = `[42.5]`
	hostMemoryJSON  = `{"total":16000000000,"available":4000000000,"used":12000000000,"usedPercent":75.0,"free":2000000000}`
	hostDiskJSON    = `{"path":"/","fstype":"ext4","total":500000000000,"free":200000000000,"used":300000000000,"usedPercent":60.0}`
	hostNetworkJSON = `[{"name":"all","bytesSent":1000000,"bytesRecv":2000000,"packetsSent":1500,"packetsRecv":2500}]`

	// Minimal docker StatsResponse; fields match container.StatsResponse JSON tags.
	// working_set = usage - stats.inactive_file = 500_000_000 - 100_000_000 = 400_000_000
	containerJSON = `{
		"name":"/node-1","id":"abc123",
		"cpu_stats":{"cpu_usage":{"total_usage":123456789000}},
		"memory_stats":{"usage":500000000,"limit":2000000000,"stats":{"inactive_file":100000000}},
		"networks":{"eth0":{"rx_bytes":50000,"tx_bytes":70000},"eth1":{"rx_bytes":3000,"tx_bytes":5000}}
	}`
)

func collectedAt() time.Time {
	return time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
}

func payload(keys map[string]string) protocol.MetricsPayload {
	data := make(map[string]json.RawMessage, len(keys))
	for k, v := range keys {
		data[k] = json.RawMessage(v)
	}
	return protocol.MetricsPayload{CollectedAt: collectedAt(), Data: data}
}

// metricNames returns sorted __name__ values for easier assertion.
func metricNames(t *testing.T, lines []vmLine) []string {
	t.Helper()
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, l.Metric["__name__"])
	}
	sort.Strings(out)
	return out
}

// findLine returns the first line whose metric map contains all key=value pairs in want.
func findLine(t *testing.T, lines []vmLine, want map[string]string) *vmLine {
	t.Helper()
	for i := range lines {
		m := lines[i].Metric
		match := true
		for k, v := range want {
			if m[k] != v {
				match = false
				break
			}
		}
		if match {
			return &lines[i]
		}
	}
	return nil
}

func TestExtractMetrics_EmptyPayload(t *testing.T) {
	lines := extractMetrics("node-1", payload(nil))
	if len(lines) != 0 {
		t.Errorf("extractMetrics(empty) = %d lines, want 0", len(lines))
	}
}

func TestExtractMetrics_HostCPU(t *testing.T) {
	lines := extractMetrics("node-1", payload(map[string]string{"cpu": hostCPUJSON}))
	if got, want := metricNames(t, lines), []string{"sentinel_host_cpu_percent"}; !slices.Equal(got, want) {
		t.Fatalf("metric names = %v, want %v", got, want)
	}
	line := lines[0]
	if line.Metric["validator"] != "node-1" {
		t.Errorf("validator label = %q, want %q", line.Metric["validator"], "node-1")
	}
	if line.Values[0] != 42.5 {
		t.Errorf("cpu percent = %v, want 42.5", line.Values[0])
	}
	if line.Timestamps[0] != collectedAt().UnixMilli() {
		t.Errorf("timestamp = %d, want %d", line.Timestamps[0], collectedAt().UnixMilli())
	}
}

func TestExtractMetrics_HostMemory(t *testing.T) {
	lines := extractMetrics("node-1", payload(map[string]string{"memory": hostMemoryJSON}))
	want := []string{
		"sentinel_host_memory_available_bytes",
		"sentinel_host_memory_free_bytes",
		"sentinel_host_memory_total_bytes",
		"sentinel_host_memory_used_bytes",
	}
	if got := metricNames(t, lines); !slices.Equal(got, want) {
		t.Fatalf("memory metric names = %v, want %v", got, want)
	}
	total := findLine(t, lines, map[string]string{"__name__": "sentinel_host_memory_total_bytes"})
	if total == nil || total.Values[0] != 16000000000 {
		t.Errorf("total = %v, want 16000000000", total)
	}
	used := findLine(t, lines, map[string]string{"__name__": "sentinel_host_memory_used_bytes"})
	if used == nil || used.Values[0] != 12000000000 {
		t.Errorf("used = %v, want 12000000000", used)
	}
}

func TestExtractMetrics_HostDisk(t *testing.T) {
	lines := extractMetrics("node-1", payload(map[string]string{"disk": hostDiskJSON}))
	want := []string{
		"sentinel_host_disk_free_bytes",
		"sentinel_host_disk_total_bytes",
		"sentinel_host_disk_used_bytes",
	}
	if got := metricNames(t, lines); !slices.Equal(got, want) {
		t.Fatalf("disk metric names = %v, want %v", got, want)
	}
	used := findLine(t, lines, map[string]string{"__name__": "sentinel_host_disk_used_bytes"})
	if used == nil {
		t.Fatal("sentinel_host_disk_used_bytes missing")
	}
	if used.Metric["path"] != "/" {
		t.Errorf("path label = %q, want /", used.Metric["path"])
	}
	if used.Metric["fstype"] != "ext4" {
		t.Errorf("fstype label = %q, want ext4", used.Metric["fstype"])
	}
	if used.Values[0] != 300000000000 {
		t.Errorf("used = %v, want 300000000000", used.Values[0])
	}
}

func TestExtractMetrics_HostNetwork(t *testing.T) {
	lines := extractMetrics("node-1", payload(map[string]string{"network": hostNetworkJSON}))
	want := []string{
		"sentinel_host_network_receive_bytes_total",
		"sentinel_host_network_transmit_bytes_total",
	}
	if got := metricNames(t, lines); !slices.Equal(got, want) {
		t.Fatalf("network metric names = %v, want %v", got, want)
	}
	recv := findLine(t, lines, map[string]string{"__name__": "sentinel_host_network_receive_bytes_total"})
	if recv == nil {
		t.Fatal("receive metric missing")
	}
	if recv.Metric["interface"] != "all" {
		t.Errorf("interface label = %q, want all", recv.Metric["interface"])
	}
	if recv.Values[0] != 2000000 {
		t.Errorf("bytesRecv = %v, want 2000000", recv.Values[0])
	}
}

func TestExtractMetrics_Container(t *testing.T) {
	lines := extractMetrics("node-1", payload(map[string]string{"container": containerJSON}))
	want := []string{
		"sentinel_container_cpu_usage_seconds_total",
		"sentinel_container_memory_limit_bytes",
		"sentinel_container_memory_usage_bytes",
		"sentinel_container_memory_working_set_bytes",
		"sentinel_container_network_receive_bytes_total",
		"sentinel_container_network_transmit_bytes_total",
	}
	if got := metricNames(t, lines); !slices.Equal(got, want) {
		t.Fatalf("container metric names = %v, want %v", got, want)
	}
	for _, l := range lines {
		if l.Metric["validator"] != "node-1" {
			t.Errorf("%s: validator = %q, want node-1", l.Metric["__name__"], l.Metric["validator"])
		}
		if l.Metric["container"] != "node-1" {
			t.Errorf("%s: container label = %q, want node-1", l.Metric["__name__"], l.Metric["container"])
		}
	}
	cpu := findLine(t, lines, map[string]string{"__name__": "sentinel_container_cpu_usage_seconds_total"})
	if cpu == nil || cpu.Values[0] != 123.456789 {
		t.Errorf("cpu seconds = %v, want 123.456789 (123456789000ns/1e9)", cpu)
	}
	ws := findLine(t, lines, map[string]string{"__name__": "sentinel_container_memory_working_set_bytes"})
	if ws == nil || ws.Values[0] != 400000000 {
		t.Errorf("working set = %v, want 400000000 (500M - 100M)", ws)
	}
	rx := findLine(t, lines, map[string]string{"__name__": "sentinel_container_network_receive_bytes_total"})
	if rx == nil || rx.Values[0] != 53000 {
		t.Errorf("rx = %v, want 53000 (50000+3000 summed across NICs)", rx)
	}
	tx := findLine(t, lines, map[string]string{"__name__": "sentinel_container_network_transmit_bytes_total"})
	if tx == nil || tx.Values[0] != 75000 {
		t.Errorf("tx = %v, want 75000 (70000+5000 summed across NICs)", tx)
	}
}

func TestExtractMetrics_Container_StripsLeadingSlashFromName(t *testing.T) {
	// Docker returns "/node-1"; we strip the leading slash for a clean label.
	lines := extractMetrics("node-1", payload(map[string]string{"container": containerJSON}))
	if len(lines) == 0 {
		t.Fatal("no lines")
	}
	for _, l := range lines {
		if l.Metric["container"] == "/node-1" {
			t.Errorf("%s: container label kept leading slash: %q", l.Metric["__name__"], l.Metric["container"])
		}
	}
}

func TestExtractMetrics_Container_FallbackNameToValidator(t *testing.T) {
	// Docker's ContainerStatsOneShot response omits Name on some API versions;
	// we fall back to the validator label so the "container" label is never empty.
	const noName = `{
		"cpu_stats":{"cpu_usage":{"total_usage":0}},
		"memory_stats":{"usage":0,"limit":0,"stats":{}},
		"networks":{}
	}`
	lines := extractMetrics("node-5", payload(map[string]string{"container": noName}))
	if len(lines) == 0 {
		t.Fatal("no lines emitted")
	}
	for _, l := range lines {
		if l.Metric["container"] != "node-5" {
			t.Errorf("%s: container = %q, want node-5 (fallback to validator)", l.Metric["__name__"], l.Metric["container"])
		}
	}
}

func TestExtractMetrics_Container_FallbackWorkingSetToUsage(t *testing.T) {
	// When stats.inactive_file is absent, working_set should fall back to usage.
	const noInactiveFile = `{
		"name":"/node-2","id":"xyz",
		"cpu_stats":{"cpu_usage":{"total_usage":0}},
		"memory_stats":{"usage":500000000,"limit":2000000000,"stats":{}},
		"networks":{}
	}`
	lines := extractMetrics("node-2", payload(map[string]string{"container": noInactiveFile}))
	ws := findLine(t, lines, map[string]string{"__name__": "sentinel_container_memory_working_set_bytes"})
	if ws == nil || ws.Values[0] != 500000000 {
		t.Errorf("working_set fallback = %v, want 500000000 (= usage)", ws)
	}
}

func TestExtractMetrics_MultipleKeys(t *testing.T) {
	lines := extractMetrics("node-1", payload(map[string]string{
		"cpu":       hostCPUJSON,
		"memory":    hostMemoryJSON,
		"container": containerJSON,
	}))
	// 1 cpu + 4 memory + 6 container = 11 lines
	if len(lines) != 11 {
		t.Errorf("got %d lines, want 11", len(lines))
	}
}

func TestExtractMetrics_MalformedJSON_SkipsKey(t *testing.T) {
	lines := extractMetrics("node-1", payload(map[string]string{
		"cpu":    hostCPUJSON,
		"memory": `not-json`,
	}))
	// cpu should still parse; memory gets silently skipped
	names := metricNames(t, lines)
	if len(names) != 1 || names[0] != "sentinel_host_cpu_percent" {
		t.Errorf("got names %v, want [sentinel_host_cpu_percent] only", names)
	}
}

func TestExtractMetrics_UnknownKey_Ignored(t *testing.T) {
	lines := extractMetrics("node-1", payload(map[string]string{"mystery_key": `{"x":1}`}))
	if len(lines) != 0 {
		t.Errorf("unknown key produced %d lines, want 0", len(lines))
	}
}

func TestExtractMetrics_MalformedContainer_SkipsBlock(t *testing.T) {
	lines := extractMetrics("node-1", payload(map[string]string{
		"cpu":       hostCPUJSON,
		"container": `{"cpu_stats": "not-an-object"}`, // shape drift
	}))
	names := metricNames(t, lines)
	if len(names) != 1 || names[0] != "sentinel_host_cpu_percent" {
		t.Errorf("got names %v, want [sentinel_host_cpu_percent] only (container should be dropped)", names)
	}
}

func TestExtractMetrics_Container_EmptyNetworks(t *testing.T) {
	const noNICs = `{
		"name":"/node-3","id":"x",
		"cpu_stats":{"cpu_usage":{"total_usage":0}},
		"memory_stats":{"usage":0,"limit":0,"stats":{}},
		"networks":{}
	}`
	lines := extractMetrics("node-3", payload(map[string]string{"container": noNICs}))
	rx := findLine(t, lines, map[string]string{"__name__": "sentinel_container_network_receive_bytes_total"})
	if rx == nil || rx.Values[0] != 0 {
		t.Errorf("rx with no NICs = %v, want 0 (present at zero)", rx)
	}
}
