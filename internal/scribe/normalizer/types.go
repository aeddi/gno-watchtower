package normalizer

import (
	"context"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/sources/loki"
	"github.com/aeddi/gno-watchtower/internal/scribe/sources/vm"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

// Lane identifies the source lane an Observation came from.
type Lane string

const (
	LaneFast Lane = "fast"
	LaneSlow Lane = "slow"
	LaneLogs Lane = "logs"
)

// Observation is a unit of input the normalizer dispatches.
// Exactly one of Metric, MetricSeries, LogEntry is populated based on Lane.
type Observation struct {
	Lane         Lane
	IngestTime   time.Time
	FromBackfill bool // when true, emitted Ops will be tagged FromBackfill
	Metric       *vm.Sample
	MetricQuery  string // PromQL that produced this Sample (for provenance)
	MetricSeries *vm.Series
	LogEntry     *loki.TailEntry
	LogQuery     string // LogQL that produced this entry (for provenance)
}

// Handler converts observations into operations.
type Handler interface {
	Name() string
	Handle(ctx context.Context, obs Observation) []types.Op
}
