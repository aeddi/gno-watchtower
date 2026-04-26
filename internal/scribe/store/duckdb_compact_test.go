package store

import (
	"context"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

func TestCompactValidatorSamplesPreservesPeaks(t *testing.T) {
	s := openTempStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Minute)

	// Three samples in the same minute bucket, varying cpu (peak in the middle).
	for i, cpu := range []float32{10, 90, 20} {
		sv := types.SampleValidator{
			ClusterID: "c1", Validator: "node-1",
			Time: base.Add(time.Duration(i) * time.Second), Tier: 0,
			Height: 100, VotingPower: 10, MempoolTxs: int32(i + 1), CPUPct: cpu, MemPct: 30,
			LastObserved: base.Add(time.Duration(i) * time.Second),
		}
		if err := s.WriteBatch(ctx, Batch{SamplesValidator: []types.SampleValidator{sv}}); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	// Compact rows older than `now - 24h`, into 1-minute buckets.
	rowsIn, rowsOut, err := s.CompactValidatorSamples(ctx, "c1", time.Now().Add(-24*time.Hour), time.Minute)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if rowsIn != 3 || rowsOut != 1 {
		t.Errorf("rowsIn=%d rowsOut=%d, want 3,1", rowsIn, rowsOut)
	}

	got, err := s.GetLatestSampleValidator(ctx, "c1", "node-1", time.Now())
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if got.Tier != 1 {
		t.Errorf("tier=%d, want 1", got.Tier)
	}
	if got.CPUPctMax == nil || *got.CPUPctMax < 89 {
		t.Errorf("cpu_pct_max not preserved: %v", got.CPUPctMax)
	}
}
