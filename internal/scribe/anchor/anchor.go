// Package anchor provides the hourly checkpoint writer that snapshots the
// in-memory live state cache into state_anchors rows, bounding the cost of
// historical projections.
package anchor

import (
	"context"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/cache"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
	"github.com/aeddi/gno-watchtower/internal/scribe/writer"
)

// Submitter is the subset of *writer.Writer the anchor needs.
type Submitter interface {
	Submit(types.Op)
}

// Anchor snapshots the live state cache for every known subject in a cluster.
type Anchor struct {
	cache   *cache.Cache
	w       Submitter
	cluster string
}

// New returns an Anchor that snapshots c for the given cluster via w.
func New(c *cache.Cache, w Submitter, cluster string) *Anchor {
	return &Anchor{cache: c, w: w, cluster: cluster}
}

// WriteOnce snapshots the current cache for every known subject in this cluster.
func (a *Anchor) WriteOnce(ctx context.Context, at time.Time) {
	for _, sub := range a.cache.Subjects(a.cluster) {
		st, ok := a.cache.Get(a.cluster, sub)
		if !ok {
			continue
		}
		fs := map[string]any{
			"peers":           st.Peers,
			"valset_view":     st.ValsetView,
			"consensus_locks": st.ConsensusLocks,
			"config_hash":     st.ConfigHash,
			"extras":          st.Extras,
		}
		a.w.Submit(types.Op{Kind: types.OpInsertAnchor, Anchor: &types.Anchor{
			ClusterID:     a.cluster,
			Subject:       sub,
			Time:          at,
			FullState:     fs,
			EventsThrough: a.cache.EventsThrough(a.cluster, sub),
		}})
	}
}

// Run loops on the configured interval until ctx is done.
func (a *Anchor) Run(ctx context.Context, interval time.Duration) error {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-t.C:
			a.WriteOnce(ctx, now)
		}
	}
}

// Keep writer import linkage so the package always builds against a known
// writer.Config shape (defensive against trimming on `goimports -w`).
var _ = writer.Config{}
