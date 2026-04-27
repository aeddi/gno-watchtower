// internal/scribe/store/store.go
package store

import (
	"context"
	"io"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

// Store is the persistence boundary. Production uses NewDuckDB; the interface
// exists to contain cgo at one layering boundary, NOT as a test substitution
// point — tests use the same DuckDB impl with t.TempDir() for isolation.
type Store interface {
	io.Closer

	// Idempotent batch writes. Implementations MUST use INSERT OR IGNORE on
	// event PK; sample upserts collapse to last-write-wins on (cluster, validator, t).
	WriteBatch(ctx context.Context, batch Batch) error

	// Range scans for the read API and projection layer.
	QueryEvents(ctx context.Context, q EventQuery) ([]types.Event, string /*next_cursor*/, error)
	GetLatestSampleValidator(ctx context.Context, cluster, validator string, at time.Time) (*types.SampleValidator, error)
	// GetMergedSampleValidator merges per-column max() across rows in a
	// `window` ending at `at`. Use this for "current scalars" — see duckdb.go.
	GetMergedSampleValidator(ctx context.Context, cluster, validator string, at time.Time, window time.Duration) (*types.SampleValidator, error)
	GetLatestSampleChain(ctx context.Context, cluster string, at time.Time) (*types.SampleChain, error)
	GetLatestAnchor(ctx context.Context, cluster, subject string, at time.Time) (*types.Anchor, error)

	// Subjects catalog (driven by min/max scans of events).
	ListSubjects(ctx context.Context, cluster string) ([]string, error)

	// Backfill job CRUD.
	UpsertBackfillJob(ctx context.Context, j BackfillJob) error
	GetBackfillJob(ctx context.Context, id string) (*BackfillJob, error)
	ListBackfillJobs(ctx context.Context, cluster string, limit int) ([]BackfillJob, error)

	// Compaction & pruning.
	CompactValidatorSamples(ctx context.Context, cluster string, before time.Time, bucket time.Duration) (rowsIn, rowsOut int64, err error)
	CompactChainSamples(ctx context.Context, cluster string, before time.Time, bucket time.Duration) (rowsIn, rowsOut int64, err error)
	PruneBefore(ctx context.Context, before time.Time) error

	// Time-bucketed samples for /api/samples.
	BucketValidatorSamples(ctx context.Context, q SamplesQuery) ([]ValidatorBucket, error)
	BucketChainSamples(ctx context.Context, q SamplesQuery) ([]ChainBucket, error)

	// Storage stats for /metrics.
	StorageBytes(ctx context.Context) (map[string]int64, error)
}

// Batch is a single transactional unit.
type Batch struct {
	Events           []types.Event
	SamplesValidator []types.SampleValidator
	SamplesChain     []types.SampleChain
	Anchors          []types.Anchor
}

type EventQuery struct {
	ClusterID string
	Subject   string // "" = no filter
	Kind      string // "" = no filter; supports prefix glob "validator.*"
	From      time.Time
	To        time.Time
	Severity  []string // "" = no filter; OR'd via SQL IN ("warning", "error", "critical")
	State     string   // "" = no filter; "open" | "recovered"
	Limit     int
	Cursor    string // event_id strict greater-than
}

type SamplesQuery struct {
	ClusterID string
	Subject   string // for validator queries; "_chain" for chain
	From      time.Time
	To        time.Time
	Step      time.Duration
}

type ValidatorBucket struct {
	T               time.Time
	Height          *int64
	VotingPower     *int64
	MempoolTxs      *float64
	MempoolTxsMax   *int64
	CPUPct          *float64
	CPUPctMax       *float64
	MemPct          *float64
	MemPctMax       *float64
	DiskPct         *float64
	NetRxBps        *float64
	NetTxBps        *float64
	PeerCountIn     *float64
	PeerCountInMin  *int64
	PeerCountOut    *float64
	PeerCountOutMin *int64
}

type ChainBucket struct {
	T                time.Time
	BlockHeight      *int64
	OnlineCount      *float64
	OnlineCountMin   *int64
	CatchingUpCount  *float64
	ValsetSize       *int64
	TotalVotingPower *int64
}

type BackfillJob struct {
	ID                    string
	ClusterID             string
	From                  time.Time
	To                    time.Time
	ChunkSize             time.Duration
	StartedAt             time.Time
	LastProgressAt        time.Time
	LastProcessedChunkEnd *time.Time
	Status                string // pending|running|completed|failed|cancelled
	ErrorCount            int
	LastError             string
}
