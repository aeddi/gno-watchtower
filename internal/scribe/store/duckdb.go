package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"

	"github.com/aeddi/gno-watchtower/internal/scribe/store/migrations"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

// duckStore is the production Store impl backed by DuckDB.
type duckStore struct {
	db *sql.DB
}

// Open creates or opens a DuckDB database at path and applies any pending migrations.
func Open(path string) (*duckStore, error) {
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("open duckdb: %w", err)
	}
	// DuckDB is single-writer in-process: cap pool to 1 to avoid lock surprises.
	db.SetMaxOpenConns(1)

	s := &duckStore{db: db}
	if err := s.applyMigrations(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *duckStore) Close() error { return s.db.Close() }

// SchemaVersion returns the highest applied migration version, or 0 if none.
func (s *duckStore) SchemaVersion(ctx context.Context) (int, error) {
	row := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version`)
	var v int
	if err := row.Scan(&v); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, nil
	}
	return v, nil
}

func (s *duckStore) applyMigrations(ctx context.Context) error {
	// Bootstrap schema_version so we can read current version before the first
	// migration runs. The first migration also creates it (IF NOT EXISTS), so
	// this is always idempotent.
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (
        version INTEGER PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL)`); err != nil {
		return fmt.Errorf("bootstrap schema_version: %w", err)
	}

	current, err := s.SchemaVersion(ctx)
	if err != nil {
		return err
	}

	for _, m := range migrations.All() {
		if m.Version <= current {
			continue
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d %s: %w", m.Version, m.Name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_version(version, applied_at) VALUES (?, ?)`,
			m.Version, time.Now().UTC()); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// ---- WriteBatch

func (s *duckStore) WriteBatch(ctx context.Context, b Batch) error {
	if len(b.Events) == 0 && len(b.SamplesValidator) == 0 && len(b.SamplesChain) == 0 && len(b.Anchors) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if len(b.Events) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
            INSERT INTO events(event_id, cluster_id, time, ingest_time, kind, subject, payload, provenance)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT (event_id) DO NOTHING`)
		if err != nil {
			return fmt.Errorf("prepare events: %w", err)
		}
		for _, e := range b.Events {
			payload, err := json.Marshal(e.Payload)
			if err != nil {
				stmt.Close()
				return fmt.Errorf("marshal payload %s: %w", e.EventID, err)
			}
			prov, err := json.Marshal(e.Provenance)
			if err != nil {
				stmt.Close()
				return fmt.Errorf("marshal provenance %s: %w", e.EventID, err)
			}
			if _, err := stmt.ExecContext(ctx,
				e.EventID, e.ClusterID, e.Time, e.IngestTime,
				e.Kind, e.Subject, string(payload), string(prov)); err != nil {
				stmt.Close()
				return fmt.Errorf("insert event %s: %w", e.EventID, err)
			}
		}
		stmt.Close()
	}

	if len(b.SamplesValidator) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
            INSERT INTO samples_validator(
                cluster_id, validator, t, tier,
                height, voting_power, catching_up,
                mempool_txs, mempool_txs_max, mempool_cached,
                cpu_pct, cpu_pct_max, mem_pct, mem_pct_max, disk_pct,
                net_rx_bps, net_tx_bps,
                peer_count_in, peer_count_in_min, peer_count_out, peer_count_out_min,
                last_observed)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT (cluster_id, validator, t) DO UPDATE SET
              height=EXCLUDED.height, voting_power=EXCLUDED.voting_power,
              catching_up=EXCLUDED.catching_up,
              mempool_txs=EXCLUDED.mempool_txs, mempool_txs_max=EXCLUDED.mempool_txs_max,
              mempool_cached=EXCLUDED.mempool_cached,
              cpu_pct=EXCLUDED.cpu_pct, cpu_pct_max=EXCLUDED.cpu_pct_max,
              mem_pct=EXCLUDED.mem_pct, mem_pct_max=EXCLUDED.mem_pct_max,
              disk_pct=EXCLUDED.disk_pct,
              net_rx_bps=EXCLUDED.net_rx_bps, net_tx_bps=EXCLUDED.net_tx_bps,
              peer_count_in=EXCLUDED.peer_count_in, peer_count_in_min=EXCLUDED.peer_count_in_min,
              peer_count_out=EXCLUDED.peer_count_out, peer_count_out_min=EXCLUDED.peer_count_out_min,
              last_observed=EXCLUDED.last_observed`)
		if err != nil {
			return fmt.Errorf("prepare samples_validator: %w", err)
		}
		for _, v := range b.SamplesValidator {
			if _, err := stmt.ExecContext(ctx,
				v.ClusterID, v.Validator, v.Time, v.Tier,
				v.Height, v.VotingPower, v.CatchingUp,
				v.MempoolTxs, v.MempoolTxsMax, v.MempoolCached,
				v.CPUPct, v.CPUPctMax, v.MemPct, v.MemPctMax, v.DiskPct,
				v.NetRxBps, v.NetTxBps,
				v.PeerCountIn, v.PeerCountInMin, v.PeerCountOut, v.PeerCountOutMin,
				v.LastObserved); err != nil {
				stmt.Close()
				return fmt.Errorf("upsert sample %s/%s: %w", v.ClusterID, v.Validator, err)
			}
		}
		stmt.Close()
	}

	if len(b.SamplesChain) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
            INSERT INTO samples_chain(
                cluster_id, t, tier,
                block_height, online_count, online_count_min, catching_up_count,
                valset_size, total_voting_power)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT (cluster_id, t) DO UPDATE SET
              block_height=EXCLUDED.block_height,
              online_count=EXCLUDED.online_count, online_count_min=EXCLUDED.online_count_min,
              catching_up_count=EXCLUDED.catching_up_count,
              valset_size=EXCLUDED.valset_size, total_voting_power=EXCLUDED.total_voting_power`)
		if err != nil {
			return fmt.Errorf("prepare samples_chain: %w", err)
		}
		for _, c := range b.SamplesChain {
			if _, err := stmt.ExecContext(ctx,
				c.ClusterID, c.Time, c.Tier,
				c.BlockHeight, c.OnlineCount, c.OnlineCountMin, c.CatchingUpCount,
				c.ValsetSize, c.TotalVotingPower); err != nil {
				stmt.Close()
				return fmt.Errorf("upsert chain sample: %w", err)
			}
		}
		stmt.Close()
	}

	if len(b.Anchors) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
            INSERT INTO state_anchors(cluster_id, subject, t, full_state, events_through)
            VALUES (?, ?, ?, ?, ?)
            ON CONFLICT (cluster_id, subject, t) DO UPDATE SET
              full_state=EXCLUDED.full_state, events_through=EXCLUDED.events_through`)
		if err != nil {
			return fmt.Errorf("prepare state_anchors: %w", err)
		}
		for _, a := range b.Anchors {
			full, err := json.Marshal(a.FullState)
			if err != nil {
				stmt.Close()
				return err
			}
			if _, err := stmt.ExecContext(ctx, a.ClusterID, a.Subject, a.Time, string(full), a.EventsThrough); err != nil {
				stmt.Close()
				return err
			}
		}
		stmt.Close()
	}

	return tx.Commit()
}

// ---- Read queries

// QueryEvents returns events matching q, ordered by event_id ASC.
// Returns a next-page cursor (non-empty when more rows exist).
func (s *duckStore) QueryEvents(ctx context.Context, q EventQuery) ([]types.Event, string, error) {
	if q.Limit <= 0 || q.Limit > 1000 {
		q.Limit = 200
	}
	args := []any{q.ClusterID}
	where := "cluster_id = ?"
	if q.Subject != "" {
		where += " AND subject = ?"
		args = append(args, q.Subject)
	}
	if q.Kind != "" {
		if last := len(q.Kind) - 1; last >= 0 && q.Kind[last] == '*' {
			where += " AND kind LIKE ?"
			args = append(args, q.Kind[:last]+"%")
		} else {
			where += " AND kind = ?"
			args = append(args, q.Kind)
		}
	}
	if !q.From.IsZero() {
		where += " AND time >= ?"
		args = append(args, q.From)
	}
	if !q.To.IsZero() {
		where += " AND time <= ?"
		args = append(args, q.To)
	}
	if q.Cursor != "" {
		where += " AND event_id > ?"
		args = append(args, q.Cursor)
	}
	args = append(args, q.Limit+1) // peek for cursor
	sqlText := `SELECT event_id, cluster_id, time, ingest_time, kind, subject, payload, provenance
	            FROM events WHERE ` + where + ` ORDER BY event_id ASC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []types.Event
	for rows.Next() {
		var e types.Event
		// DuckDB JSON columns return parsed values (map/slice), not raw strings.
		// Scan into any, re-marshal to bytes, then unmarshal into the typed fields.
		var payload, prov any
		if err := rows.Scan(&e.EventID, &e.ClusterID, &e.Time, &e.IngestTime,
			&e.Kind, &e.Subject, &payload, &prov); err != nil {
			return nil, "", err
		}
		if b, err := json.Marshal(payload); err == nil {
			_ = json.Unmarshal(b, &e.Payload)
		}
		if b, err := json.Marshal(prov); err == nil {
			_ = json.Unmarshal(b, &e.Provenance)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	var nextCursor string
	if len(out) > q.Limit {
		nextCursor = out[q.Limit-1].EventID
		out = out[:q.Limit]
	}
	return out, nextCursor, nil
}

// GetLatestSampleValidator returns the most recent sample for (cluster, validator)
// at or before at, or nil if none exists.
func (s *duckStore) GetLatestSampleValidator(ctx context.Context, cluster, validator string, at time.Time) (*types.SampleValidator, error) {
	row := s.db.QueryRowContext(ctx, `
        SELECT cluster_id, validator, t, tier,
               height, voting_power, catching_up,
               mempool_txs, mempool_txs_max, mempool_cached,
               cpu_pct, cpu_pct_max, mem_pct, mem_pct_max, disk_pct,
               net_rx_bps, net_tx_bps,
               peer_count_in, peer_count_in_min, peer_count_out, peer_count_out_min,
               last_observed
        FROM samples_validator
        WHERE cluster_id = ? AND validator = ? AND t <= ?
        ORDER BY t DESC LIMIT 1`, cluster, validator, at)
	var v types.SampleValidator
	if err := row.Scan(&v.ClusterID, &v.Validator, &v.Time, &v.Tier,
		&v.Height, &v.VotingPower, &v.CatchingUp,
		&v.MempoolTxs, &v.MempoolTxsMax, &v.MempoolCached,
		&v.CPUPct, &v.CPUPctMax, &v.MemPct, &v.MemPctMax, &v.DiskPct,
		&v.NetRxBps, &v.NetTxBps,
		&v.PeerCountIn, &v.PeerCountInMin, &v.PeerCountOut, &v.PeerCountOutMin,
		&v.LastObserved); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}
