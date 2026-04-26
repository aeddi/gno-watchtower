package compactor

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

func TestRunOnceCompactsAndPrunes(t *testing.T) {
	s, _ := store.Open(filepath.Join(t.TempDir(), "scribe.duckdb"))
	defer s.Close()
	ctx := context.Background()
	old := time.Now().Add(-31 * 24 * time.Hour).UTC().Truncate(time.Minute)
	for i := 0; i < 3; i++ {
		_ = s.WriteBatch(ctx, store.Batch{SamplesValidator: []types.SampleValidator{{
			ClusterID: "c1", Validator: "node-1", Time: old.Add(time.Duration(i) * time.Second),
			Tier: 0, Height: 100, CPUPct: float32(10 + i), LastObserved: old,
		}}})
	}
	c := New(s, "c1", Config{
		HotWindow:  30 * 24 * time.Hour,
		WarmBucket: time.Minute,
		PruneAfter: 365 * 24 * time.Hour,
	})
	if err := c.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	got, _ := s.GetLatestSampleValidator(ctx, "c1", "node-1", time.Now())
	if got == nil || got.Tier != 1 {
		t.Errorf("expected warm row after compact: %+v", got)
	}
}
