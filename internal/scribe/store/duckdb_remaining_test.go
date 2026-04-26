package store

import (
	"context"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

func TestGetMergedSampleValidatorMergesPerHandlerRows(t *testing.T) {
	s := openTempStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Three "handlers" each write a row at the same logical tick but with
	// staggered µs offsets — each only sets its own column.
	rows := []types.SampleValidator{
		{ClusterID: "c1", Validator: "node-1", Time: now.Add(1 * time.Microsecond), Tier: 0, Height: 12345, LastObserved: now},
		{ClusterID: "c1", Validator: "node-1", Time: now.Add(2 * time.Microsecond), Tier: 0, MempoolTxs: 7},
		{ClusterID: "c1", Validator: "node-1", Time: now.Add(3 * time.Microsecond), Tier: 0, VotingPower: 42},
	}
	for i := range rows {
		if err := s.WriteBatch(ctx, Batch{SamplesValidator: []types.SampleValidator{rows[i]}}); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	got, err := s.GetMergedSampleValidator(ctx, "c1", "node-1", now.Add(time.Second), time.Minute)
	if err != nil {
		t.Fatalf("GetMergedSampleValidator: %v", err)
	}
	if got == nil {
		t.Fatal("got nil")
	}
	if got.Height != 12345 {
		t.Errorf("Height = %d, want 12345", got.Height)
	}
	if got.MempoolTxs != 7 {
		t.Errorf("MempoolTxs = %d, want 7", got.MempoolTxs)
	}
	if got.VotingPower != 42 {
		t.Errorf("VotingPower = %d, want 42", got.VotingPower)
	}

	// Outside-window query returns nil.
	stale, err := s.GetMergedSampleValidator(ctx, "c1", "node-1", now.Add(2*time.Hour), 30*time.Second)
	if err != nil {
		t.Fatalf("stale: %v", err)
	}
	if stale != nil {
		t.Errorf("expected nil for query outside window, got %+v", stale)
	}
}

func TestGetLatestAnchorAndChain(t *testing.T) {
	s := openTempStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	a := types.Anchor{
		ClusterID: "c1", Subject: "node-1", Time: now,
		FullState:     map[string]any{"peers": []any{}},
		EventsThrough: "01JCT0AAA0AAA0AAA0AAA0AAA0",
	}
	if err := s.WriteBatch(ctx, Batch{Anchors: []types.Anchor{a}}); err != nil {
		t.Fatalf("write anchor: %v", err)
	}
	got, err := s.GetLatestAnchor(ctx, "c1", "node-1", now.Add(time.Minute))
	if err != nil || got == nil || got.EventsThrough != a.EventsThrough {
		t.Fatalf("GetLatestAnchor: got=%+v err=%v", got, err)
	}

	sc := types.SampleChain{ClusterID: "c1", Time: now, BlockHeight: 100, OnlineCount: 4, ValsetSize: 4, TotalVotingPower: 400}
	if err := s.WriteBatch(ctx, Batch{SamplesChain: []types.SampleChain{sc}}); err != nil {
		t.Fatalf("write chain sample: %v", err)
	}
	gc, err := s.GetLatestSampleChain(ctx, "c1", now.Add(time.Minute))
	if err != nil || gc == nil || gc.BlockHeight != 100 {
		t.Fatalf("GetLatestSampleChain: got=%+v err=%v", gc, err)
	}
}

func TestListSubjectsDistinct(t *testing.T) {
	s := openTempStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, sub := range []string{"node-1", "node-2", "_chain", "node-1"} {
		// Pad to 26-char ULID per row; same time → unique IDs by mixing in subject.
		ev := types.Event{
			EventID:   ("01JCT" + sub + "00000000000000000000000000")[:26],
			ClusterID: "c1", Time: now, IngestTime: now,
			Kind: "x", Subject: sub,
			Payload: map[string]any{}, Provenance: types.Provenance{Type: types.ProvenanceMetric},
		}
		_ = s.WriteBatch(ctx, Batch{Events: []types.Event{ev}})
	}
	subs, err := s.ListSubjects(ctx, "c1")
	if err != nil {
		t.Fatalf("ListSubjects: %v", err)
	}
	if len(subs) != 3 {
		t.Errorf("got %v, want 3 distinct", subs)
	}
}

func TestStorageBytesReturnsTables(t *testing.T) {
	s := openTempStore(t)
	got, err := s.StorageBytes(context.Background())
	if err != nil {
		t.Fatalf("StorageBytes: %v", err)
	}
	for _, want := range []string{"events", "samples_validator", "samples_chain", "state_anchors"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing %q in StorageBytes result %+v", want, got)
		}
	}
}
