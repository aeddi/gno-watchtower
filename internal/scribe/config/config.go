// Package config loads and provides the scribe TOML configuration.
package config

import (
	_ "embed"
	"fmt"
	"os"
	"time"

	"github.com/pelletier/go-toml/v2"
)

//go:embed default.toml
var defaultTOML []byte

// Config is the top-level scribe configuration tree.
type Config struct {
	Server    Server    `toml:"server"`
	Cluster   Cluster   `toml:"cluster"`
	Storage   Storage   `toml:"storage"`
	Sources   Sources   `toml:"sources"`
	Ingest    Ingest    `toml:"ingest"`
	Writer    Writer    `toml:"writer"`
	Anchors   Anchors   `toml:"anchors"`
	Retention Retention `toml:"retention"`
	Backfill  Backfill  `toml:"backfill"`
	SSE       SSE       `toml:"sse"`
	Logging   Logging   `toml:"logging"`
}

// Server holds HTTP server settings.
type Server struct {
	ListenAddr string `toml:"listen_addr"`
}

// Cluster holds cluster identity settings.
type Cluster struct {
	ID string `toml:"id"`
}

// Storage holds DuckDB path settings.
type Storage struct {
	DBPath string `toml:"db_path"`
}

// Sources holds upstream data source endpoints.
type Sources struct {
	VM   Endpoint `toml:"victoria_metrics"`
	Loki Endpoint `toml:"loki"`
}

// Endpoint holds a single URL.
type Endpoint struct {
	URL string `toml:"url"`
}

// Ingest holds ingestion lane configuration.
type Ingest struct {
	Fast  IngestFast  `toml:"fast"`
	Slow  IngestSlow  `toml:"slow"`
	Logs  IngestLogs  `toml:"logs"`
	Lanes IngestLanes `toml:"lanes"`
}

// IngestFast holds fast-lane PromQL ingest settings.
type IngestFast struct {
	Interval Duration `toml:"interval"`
	Queries  []string `toml:"queries"`
}

// IngestSlow holds slow-lane PromQL ingest settings.
type IngestSlow struct {
	Interval Duration `toml:"interval"`
	Queries  []string `toml:"queries"`
}

// IngestLogs holds Loki tail ingest settings.
type IngestLogs struct {
	Streams       []string `toml:"streams"`
	OverlapWindow Duration `toml:"overlap_window"`
}

// IngestLanes holds shared lane back-pressure settings.
type IngestLanes struct {
	BufferSlots    int      `toml:"buffer_slots"`
	BackoffInitial Duration `toml:"backoff_initial"`
	BackoffMax     Duration `toml:"backoff_max"`
}

// Writer holds batch-write settings.
type Writer struct {
	BatchSize   int      `toml:"batch_size"`
	BatchWindow Duration `toml:"batch_window"`
}

// Anchors holds anchor-write scheduling settings.
type Anchors struct {
	Interval Duration `toml:"interval"`
}

// Retention holds data retention and compaction settings.
type Retention struct {
	HotWindow     Duration `toml:"hot_window"`
	WarmWindow    Duration `toml:"warm_window"`
	WarmBucket    Duration `toml:"warm_bucket"`
	PruneAfter    Duration `toml:"prune_after"`
	CompactAt     string   `toml:"compact_at"`
	CompactJitter Duration `toml:"compact_jitter"`
}

// Backfill holds backfill engine settings.
type Backfill struct {
	ChunkSize        Duration `toml:"chunk_size"`
	DefaultLookback  Duration `toml:"default_lookback"`
	ResumeStaleAfter Duration `toml:"resume_stale_after"`
}

// SSE holds server-sent event stream settings.
type SSE struct {
	SlowSubscriberTimeout Duration `toml:"slow_subscriber_timeout"`
	MaxSubscribers        int      `toml:"max_subscribers"`
}

// Logging holds log output settings.
type Logging struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
}

// Duration wraps time.Duration to parse TOML duration strings ("3s", "30d") and
// support the "0" sentinel as time.Duration(0).
type Duration time.Duration

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Duration) UnmarshalText(text []byte) error {
	s := string(text)
	if s == "0" || s == "" {
		*d = 0
		return nil
	}
	// Allow "30d" / "365d" — Go's stdlib does not support day suffixes.
	if l := len(s); l > 1 && (s[l-1] == 'd' || s[l-1] == 'D') {
		hours, err := time.ParseDuration(s[:l-1] + "h")
		if err != nil {
			return fmt.Errorf("parse %q: %w", s, err)
		}
		*d = Duration(hours * 24)
		return nil
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// Std returns the underlying time.Duration.
func (d Duration) Std() time.Duration { return time.Duration(d) }

// Load reads and parses the TOML file at path.
func Load(path string) (*Config, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Config{}
	if err := toml.Unmarshal(body, c); err != nil {
		return nil, err
	}
	return c, nil
}

// DefaultTOML returns the embedded default configuration bytes.
func DefaultTOML() ([]byte, error) { return defaultTOML, nil }
