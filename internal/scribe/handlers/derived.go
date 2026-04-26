package handlers

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/eventid"
	"github.com/aeddi/gno-watchtower/internal/scribe/kinds"
	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
	"github.com/aeddi/gno-watchtower/internal/scribe/writer"
)

// VoteMissed emits `validator.vote_missed` for every validator that did not
// produce a `validator.vote_cast` for a committed height. Commit-anchored:
// reacts when `chain.block_committed{height=H}` lands.
type VoteMissed struct {
	cluster    string
	validators []string
	store      store.Store
	writer     *writer.Writer
}

func NewVoteMissed(cluster string, validators []string, s store.Store, w *writer.Writer) *VoteMissed {
	return &VoteMissed{cluster: cluster, validators: validators, store: s, writer: w}
}

func (v *VoteMissed) Run(ctx context.Context) error {
	sub := v.writer.Subscribe(64)
	defer v.writer.Unsubscribe(sub)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-sub:
			if !ok {
				return nil
			}
			if ev.Kind != "chain.block_committed" {
				continue
			}
			height := readPayloadInt64(ev.Payload, "height")
			round := readPayloadInt32(ev.Payload, "round")
			v.checkHeight(ctx, ev, height, round)
		}
	}
}

func (v *VoteMissed) checkHeight(ctx context.Context, committed types.Event, height int64, round int32) {
	for _, val := range v.validators {
		evs, _, err := v.store.QueryEvents(ctx, store.EventQuery{
			ClusterID: v.cluster, Subject: val, Kind: "validator.vote_cast",
			Limit: 50,
		})
		if err != nil {
			continue
		}
		voted := false
		for _, e := range evs {
			h := readPayloadInt64(e.Payload, "height")
			if h == height {
				voted = true
				break
			}
		}
		if voted {
			continue
		}
		payload := kinds.ValidatorVoteMissed{Height: height, Round: round, VoteType: "precommit"}
		pb, _ := json.Marshal(payload)
		ev := types.Event{
			EventID:    eventid.Derive(committed.Time, payload.Kind(), val, pb),
			ClusterID:  v.cluster,
			Time:       committed.Time,
			IngestTime: committed.IngestTime,
			Kind:       payload.Kind(),
			Subject:    val,
			Payload:    map[string]any{"height": height, "round": round, "vote_type": "precommit"},
			Provenance: types.Provenance{
				Type: types.ProvenanceDerived, Rule: "vote_missed_v1",
				Inputs: []types.DerivationIn{
					{EventID: committed.EventID, Kind: "chain.block_committed", Extras: map[string]any{"height": height}},
					{AbsenceOf: "validator.vote_cast", Subject: val},
				},
				Queries: []types.ProvenanceQ{
					{Backend: "loki", Q: `{validator="` + val + `"} |= "Precommit"`},
					{Backend: "vm", Q: `sentinel_rpc_latest_block_height{validator="` + val + `"}`},
				},
			},
		}
		v.writer.Submit(types.Op{Kind: types.OpInsertEvent, Event: &ev})
	}
}

// SignedBlock — TODO: subscribe to chain.block_committed, look up valset, emit
// validator.signed_block for each valset member assumed to have signed. Stub for
// now; revisit after Phase 14 replay tests inform what data is actually available.
type SignedBlock struct {
	cluster string
	store   store.Store
	writer  *writer.Writer
}

func NewSignedBlock(cluster string, s store.Store, w *writer.Writer) *SignedBlock {
	return &SignedBlock{cluster: cluster, store: s, writer: w}
}

func (b *SignedBlock) Run(ctx context.Context) error {
	sub := b.writer.Subscribe(16)
	defer b.writer.Unsubscribe(sub)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-sub:
			if !ok {
				return nil
			}
			// TODO(phase-14): emit validator.signed_block events from valset metadata.
		}
	}
}

// ConsensusStuck — TODO: track latest chain.block_committed time vs round-step
// progression; emit chain.consensus_stuck if rounds advance for >N seconds with
// no new block. Stub for now.
type ConsensusStuck struct {
	cluster     string
	writer      *writer.Writer
	mu          sync.Mutex
	lastBlockAt time.Time
}

func NewConsensusStuck(cluster string, w *writer.Writer) *ConsensusStuck {
	return &ConsensusStuck{cluster: cluster, writer: w}
}

func (c *ConsensusStuck) Run(ctx context.Context) error {
	sub := c.writer.Subscribe(16)
	defer c.writer.Unsubscribe(sub)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-sub:
			if !ok {
				return nil
			}
			if ev.Kind == "chain.block_committed" {
				c.mu.Lock()
				c.lastBlockAt = ev.Time
				c.mu.Unlock()
			}
			// TODO(phase-14): emit chain.consensus_stuck on round-step progression
			// without a corresponding block_committed within the configured timeout.
		}
	}
}

// readPayloadInt64 and readPayloadInt32 coerce JSON-decoded payload fields to
// integer types. JSON numbers decode as float64 by default; stored int64/int32
// values are passed through directly.
func readPayloadInt64(m map[string]any, k string) int64 {
	switch v := m[k].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int32:
		return int64(v)
	case int:
		return int64(v)
	default:
		return 0
	}
}

func readPayloadInt32(m map[string]any, k string) int32 {
	return int32(readPayloadInt64(m, k))
}
