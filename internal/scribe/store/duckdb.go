package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
            INSERT INTO events(event_id, cluster_id, time, ingest_time, kind, subject,
                               severity, state, recovers, payload, provenance)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			sev := nullableString(e.Severity)
			st := nullableString(e.State)
			rec := nullableString(e.Recovers)
			if _, err := stmt.ExecContext(ctx,
				e.EventID, e.ClusterID, e.Time, e.IngestTime,
				e.Kind, e.Subject, sev, st, rec,
				string(payload), string(prov)); err != nil {
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
                behind_sentry, last_observed)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
              behind_sentry=COALESCE(EXCLUDED.behind_sentry, samples_validator.behind_sentry),
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
				v.BehindSentry, v.LastObserved); err != nil {
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
	if len(q.Severity) > 0 {
		placeholders := make([]string, len(q.Severity))
		for i, s := range q.Severity {
			placeholders[i] = "?"
			args = append(args, s)
		}
		where += " AND severity IN (" + strings.Join(placeholders, ",") + ")"
	}
	if q.State != "" {
		where += " AND state = ?"
		args = append(args, q.State)
	}
	args = append(args, q.Limit+1) // peek for cursor
	sqlText := `SELECT event_id, cluster_id, time, ingest_time, kind, subject,
                       severity, state, recovers, payload, provenance
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
		var sev, st, rec sql.NullString
		if err := rows.Scan(&e.EventID, &e.ClusterID, &e.Time, &e.IngestTime,
			&e.Kind, &e.Subject, &sev, &st, &rec, &payload, &prov); err != nil {
			return nil, "", err
		}
		if sev.Valid {
			e.Severity = sev.String
		}
		if st.Valid {
			e.State = st.String
		}
		if rec.Valid {
			e.Recovers = rec.String
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

// ---- Remaining Store interface methods

func (s *duckStore) GetLatestSampleChain(ctx context.Context, cluster string, at time.Time) (*types.SampleChain, error) {
	row := s.db.QueryRowContext(ctx, `
        SELECT cluster_id, t, tier, block_height, online_count, online_count_min,
               catching_up_count, valset_size, total_voting_power
        FROM samples_chain WHERE cluster_id = ? AND t <= ?
        ORDER BY t DESC LIMIT 1`, cluster, at)
	var c types.SampleChain
	if err := row.Scan(&c.ClusterID, &c.Time, &c.Tier,
		&c.BlockHeight, &c.OnlineCount, &c.OnlineCountMin,
		&c.CatchingUpCount, &c.ValsetSize, &c.TotalVotingPower); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (s *duckStore) GetLatestAnchor(ctx context.Context, cluster, subject string, at time.Time) (*types.Anchor, error) {
	row := s.db.QueryRowContext(ctx, `
        SELECT cluster_id, subject, t, full_state, events_through
        FROM state_anchors WHERE cluster_id = ? AND subject = ? AND t <= ?
        ORDER BY t DESC LIMIT 1`, cluster, subject, at)
	var a types.Anchor
	var full any // DuckDB JSON column: arrives as map/slice already; scan into any then re-roundtrip.
	if err := row.Scan(&a.ClusterID, &a.Subject, &a.Time, &full, &a.EventsThrough); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if full != nil {
		b, err := json.Marshal(full)
		if err != nil {
			return nil, fmt.Errorf("marshal full_state: %w", err)
		}
		if err := json.Unmarshal(b, &a.FullState); err != nil {
			return nil, fmt.Errorf("unmarshal full_state: %w", err)
		}
	}
	return &a, nil
}

func (s *duckStore) ListSubjects(ctx context.Context, cluster string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT subject FROM events WHERE cluster_id = ? ORDER BY subject`, cluster)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var sub string
		if err := rows.Scan(&sub); err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (s *duckStore) UpsertBackfillJob(ctx context.Context, j BackfillJob) error {
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO backfill_jobs(id, cluster_id, range_from, range_to, chunk_size_seconds,
            started_at, last_progress_at, last_processed_chunk_end, status, error_count, last_error)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT (id) DO UPDATE SET
          last_progress_at=EXCLUDED.last_progress_at,
          last_processed_chunk_end=EXCLUDED.last_processed_chunk_end,
          status=EXCLUDED.status,
          error_count=EXCLUDED.error_count,
          last_error=EXCLUDED.last_error`,
		j.ID, j.ClusterID, j.From, j.To, int(j.ChunkSize.Seconds()),
		j.StartedAt, j.LastProgressAt, j.LastProcessedChunkEnd,
		j.Status, j.ErrorCount, j.LastError)
	return err
}

func (s *duckStore) GetBackfillJob(ctx context.Context, id string) (*BackfillJob, error) {
	row := s.db.QueryRowContext(ctx, `
        SELECT id, cluster_id, range_from, range_to, chunk_size_seconds,
               started_at, last_progress_at, last_processed_chunk_end,
               status, error_count, COALESCE(last_error,'')
        FROM backfill_jobs WHERE id = ?`, id)
	var j BackfillJob
	var chunkSec int
	if err := row.Scan(&j.ID, &j.ClusterID, &j.From, &j.To, &chunkSec,
		&j.StartedAt, &j.LastProgressAt, &j.LastProcessedChunkEnd,
		&j.Status, &j.ErrorCount, &j.LastError); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	j.ChunkSize = time.Duration(chunkSec) * time.Second
	return &j, nil
}

func (s *duckStore) ListBackfillJobs(ctx context.Context, cluster string, limit int) ([]BackfillJob, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
        SELECT id, cluster_id, range_from, range_to, chunk_size_seconds,
               started_at, last_progress_at, last_processed_chunk_end,
               status, error_count, COALESCE(last_error,'')
        FROM backfill_jobs WHERE cluster_id = ?
        ORDER BY started_at DESC LIMIT ?`, cluster, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BackfillJob
	for rows.Next() {
		var j BackfillJob
		var chunkSec int
		if err := rows.Scan(&j.ID, &j.ClusterID, &j.From, &j.To, &chunkSec,
			&j.StartedAt, &j.LastProgressAt, &j.LastProcessedChunkEnd,
			&j.Status, &j.ErrorCount, &j.LastError); err != nil {
			return nil, err
		}
		j.ChunkSize = time.Duration(chunkSec) * time.Second
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *duckStore) PruneBefore(ctx context.Context, before time.Time) error {
	for _, q := range []string{
		`DELETE FROM events WHERE time < ?`,
		`DELETE FROM samples_validator WHERE t < ?`,
		`DELETE FROM samples_chain WHERE t < ?`,
		`DELETE FROM state_anchors WHERE t < ?`,
	} {
		if _, err := s.db.ExecContext(ctx, q, before); err != nil {
			return fmt.Errorf("prune %s: %w", q, err)
		}
	}
	return nil
}

func (s *duckStore) StorageBytes(ctx context.Context) (map[string]int64, error) {
	out := map[string]int64{}
	for _, t := range []string{"events", "samples_validator", "samples_chain", "state_anchors", "backfill_jobs"} {
		var n int64
		if err := s.db.QueryRowContext(ctx,
			fmt.Sprintf("SELECT COUNT(*) FROM %s", t)).Scan(&n); err != nil {
			return nil, err
		}
		out[t] = n // row count is the closest cheap proxy; on-disk bytes via PRAGMA database_size if needed later.
	}
	return out, nil
}

// ---- Helpers

// nullableString returns sql.NullString for empty-string-as-NULL semantics.
// Used for the analysis columns severity/state/recovers, which are NULL on
// non-diagnostic events.
func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// ---- Compaction

func (s *duckStore) CompactValidatorSamples(ctx context.Context, cluster string, before time.Time, bucket time.Duration) (int64, int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var rowsIn int64
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM samples_validator WHERE cluster_id = ? AND tier = 0 AND t < ?`,
		cluster, before).Scan(&rowsIn); err != nil {
		return 0, 0, err
	}
	if rowsIn == 0 {
		return 0, 0, tx.Commit()
	}

	bucketSecs := int64(bucket.Seconds())
	insertQ := fmt.Sprintf(`
        INSERT INTO samples_validator
        SELECT cluster_id, validator,
               TIMESTAMP 'epoch' + (CAST(epoch(t)/%d AS BIGINT) * %d) * INTERVAL '1 second' AS t,
               1 AS tier,
               max(height), arg_max(voting_power, t),
               arg_max(catching_up, t),
               CAST(avg(mempool_txs) AS INTEGER), max(mempool_txs),
               CAST(avg(mempool_cached) AS INTEGER),
               avg(cpu_pct), max(cpu_pct),
               avg(mem_pct), max(mem_pct),
               avg(disk_pct), avg(net_rx_bps), avg(net_tx_bps),
               CAST(avg(peer_count_in) AS SMALLINT), min(peer_count_in),
               CAST(avg(peer_count_out) AS SMALLINT), min(peer_count_out),
               max(last_observed),
               bool_or(behind_sentry)
        FROM samples_validator
        WHERE cluster_id = ? AND tier = 0 AND t < ?
        GROUP BY cluster_id, validator,
                 CAST(epoch(t)/%d AS BIGINT)
        ON CONFLICT (cluster_id, validator, t) DO UPDATE SET
              tier = 1, height = EXCLUDED.height, voting_power = EXCLUDED.voting_power,
              catching_up = EXCLUDED.catching_up,
              mempool_txs = EXCLUDED.mempool_txs, mempool_txs_max = EXCLUDED.mempool_txs_max,
              mempool_cached = EXCLUDED.mempool_cached,
              cpu_pct = EXCLUDED.cpu_pct, cpu_pct_max = EXCLUDED.cpu_pct_max,
              mem_pct = EXCLUDED.mem_pct, mem_pct_max = EXCLUDED.mem_pct_max,
              disk_pct = EXCLUDED.disk_pct,
              net_rx_bps = EXCLUDED.net_rx_bps, net_tx_bps = EXCLUDED.net_tx_bps,
              peer_count_in = EXCLUDED.peer_count_in, peer_count_in_min = EXCLUDED.peer_count_in_min,
              peer_count_out = EXCLUDED.peer_count_out, peer_count_out_min = EXCLUDED.peer_count_out_min,
              behind_sentry = COALESCE(EXCLUDED.behind_sentry, samples_validator.behind_sentry),
              last_observed = EXCLUDED.last_observed`,
		bucketSecs, bucketSecs, bucketSecs)
	res, err := tx.ExecContext(ctx, insertQ, cluster, before)
	if err != nil {
		return 0, 0, fmt.Errorf("compact insert: %w", err)
	}
	rowsOut, _ := res.RowsAffected()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM samples_validator WHERE cluster_id = ? AND tier = 0 AND t < ?`,
		cluster, before); err != nil {
		return 0, 0, fmt.Errorf("compact delete: %w", err)
	}

	return rowsIn, rowsOut, tx.Commit()
}

func (s *duckStore) CompactChainSamples(ctx context.Context, cluster string, before time.Time, bucket time.Duration) (int64, int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var rowsIn int64
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM samples_chain WHERE cluster_id = ? AND tier = 0 AND t < ?`,
		cluster, before).Scan(&rowsIn); err != nil {
		return 0, 0, err
	}
	if rowsIn == 0 {
		return 0, 0, tx.Commit()
	}

	bs := int64(bucket.Seconds())
	insertQ := fmt.Sprintf(`
        INSERT INTO samples_chain
        SELECT cluster_id,
               TIMESTAMP 'epoch' + (CAST(epoch(t)/%d AS BIGINT) * %d) * INTERVAL '1 second' AS t,
               1, max(block_height),
               CAST(avg(online_count) AS SMALLINT), min(online_count),
               CAST(avg(catching_up_count) AS SMALLINT),
               arg_max(valset_size, t), arg_max(total_voting_power, t)
        FROM samples_chain
        WHERE cluster_id = ? AND tier = 0 AND t < ?
        GROUP BY cluster_id, CAST(epoch(t)/%d AS BIGINT)
        ON CONFLICT (cluster_id, t) DO UPDATE SET
          tier=1, block_height=EXCLUDED.block_height,
          online_count=EXCLUDED.online_count, online_count_min=EXCLUDED.online_count_min,
          catching_up_count=EXCLUDED.catching_up_count,
          valset_size=EXCLUDED.valset_size, total_voting_power=EXCLUDED.total_voting_power`,
		bs, bs, bs)
	res, err := tx.ExecContext(ctx, insertQ, cluster, before)
	if err != nil {
		return 0, 0, err
	}
	rowsOut, _ := res.RowsAffected()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM samples_chain WHERE cluster_id = ? AND tier = 0 AND t < ?`, cluster, before); err != nil {
		return 0, 0, err
	}

	return rowsIn, rowsOut, tx.Commit()
}

// ---- Time-bucketed sample queries

func (s *duckStore) BucketValidatorSamples(ctx context.Context, q SamplesQuery) ([]ValidatorBucket, error) {
	bs := int64(q.Step.Seconds())
	if bs <= 0 {
		bs = 60
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
        SELECT TIMESTAMP 'epoch' + (CAST(epoch(t)/%d AS BIGINT) * %d) * INTERVAL '1 second' AS bt,
               max(height), arg_max(voting_power, t),
               avg(mempool_txs), max(mempool_txs),
               avg(cpu_pct), max(cpu_pct), avg(mem_pct), max(mem_pct),
               avg(disk_pct), avg(net_rx_bps), avg(net_tx_bps),
               avg(peer_count_in), min(peer_count_in),
               avg(peer_count_out), min(peer_count_out)
        FROM samples_validator
        WHERE cluster_id = ? AND validator = ? AND t >= ? AND t <= ?
        GROUP BY bt ORDER BY bt`, bs, bs),
		q.ClusterID, q.Subject, q.From, q.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ValidatorBucket
	for rows.Next() {
		var b ValidatorBucket
		if err := rows.Scan(&b.T, &b.Height, &b.VotingPower,
			&b.MempoolTxs, &b.MempoolTxsMax,
			&b.CPUPct, &b.CPUPctMax, &b.MemPct, &b.MemPctMax,
			&b.DiskPct, &b.NetRxBps, &b.NetTxBps,
			&b.PeerCountIn, &b.PeerCountInMin,
			&b.PeerCountOut, &b.PeerCountOutMin); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *duckStore) BucketChainSamples(ctx context.Context, q SamplesQuery) ([]ChainBucket, error) {
	bs := int64(q.Step.Seconds())
	if bs <= 0 {
		bs = 60
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
        SELECT TIMESTAMP 'epoch' + (CAST(epoch(t)/%d AS BIGINT) * %d) * INTERVAL '1 second' AS bt,
               max(block_height), avg(online_count), min(online_count),
               avg(catching_up_count), arg_max(valset_size, t), arg_max(total_voting_power, t)
        FROM samples_chain WHERE cluster_id = ? AND t >= ? AND t <= ?
        GROUP BY bt ORDER BY bt`, bs, bs),
		q.ClusterID, q.From, q.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChainBucket
	for rows.Next() {
		var b ChainBucket
		if err := rows.Scan(&b.T, &b.BlockHeight, &b.OnlineCount, &b.OnlineCountMin,
			&b.CatchingUpCount, &b.ValsetSize, &b.TotalVotingPower); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetLatestSampleValidator returns the most recent sample row for
// (cluster, validator) at or before `at`, or nil if none exists.
// Use GetMergedSampleValidator when you need merged-across-handlers values
// (most fast_scalars use cases want that).
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

// GetMergedSampleValidator returns a per-column merge of the most recent
// samples for (cluster, validator) over a `window` ending at `at`.
//
// Why merge: per-handler sample writes stagger their `t` by a few µs to avoid
// PK collisions, and each handler only populates its own scalar column (zeros
// for the others). A naive "latest row" returns whichever handler wrote last
// and drops every other column to zero. Aggregating `max()` per column gives
// every column its real value (handlers other than the column's owner always
// have 0, so max picks the owner's real value).
//
// Returns nil when no rows exist in the window.
func (s *duckStore) GetMergedSampleValidator(ctx context.Context, cluster, validator string, at time.Time, window time.Duration) (*types.SampleValidator, error) {
	if window <= 0 {
		window = 30 * time.Second
	}
	row := s.db.QueryRowContext(ctx, `
        SELECT max(t), max(tier),
               max(height), max(voting_power), bool_or(catching_up),
               max(mempool_txs), max(mempool_txs_max), max(mempool_cached),
               max(cpu_pct), max(cpu_pct_max), max(mem_pct), max(mem_pct_max), max(disk_pct),
               max(net_rx_bps), max(net_tx_bps),
               max(peer_count_in), max(peer_count_in_min),
               max(peer_count_out), max(peer_count_out_min),
               bool_or(behind_sentry),
               max(last_observed)
        FROM samples_validator
        WHERE cluster_id = ? AND validator = ? AND t <= ? AND t > ?`,
		cluster, validator, at, at.Add(-window))
	// Aggregate queries always return one row even when there are no source
	// rows — every column comes back NULL in that case. So scan everything into
	// pointer/nullable values, then collapse to nil if `t` (the row's anchor)
	// is NULL.
	var (
		t            *time.Time
		tier         *int8
		height       *int64
		votingPower  *int64
		catching     *bool
		mempool      *int32
		mempoolMax   *int32
		mempoolCach  *int32
		cpu          *float32
		cpuMax       *float32
		mem          *float32
		memMax       *float32
		disk         *float32
		netRx        *float32
		netTx        *float32
		peerIn       *int16
		peerInMin    *int16
		peerOut      *int16
		peerOutMin   *int16
		behindSentry sql.NullBool
		lastObserved *time.Time
	)
	if err := row.Scan(&t, &tier,
		&height, &votingPower, &catching,
		&mempool, &mempoolMax, &mempoolCach,
		&cpu, &cpuMax, &mem, &memMax, &disk,
		&netRx, &netTx,
		&peerIn, &peerInMin, &peerOut, &peerOutMin,
		&behindSentry, &lastObserved); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if t == nil {
		return nil, nil
	}
	v := types.SampleValidator{ClusterID: cluster, Validator: validator, Time: *t}
	if tier != nil {
		v.Tier = *tier
	}
	if height != nil {
		v.Height = *height
	}
	if votingPower != nil {
		v.VotingPower = *votingPower
	}
	if catching != nil {
		v.CatchingUp = *catching
	}
	if mempool != nil {
		v.MempoolTxs = *mempool
	}
	v.MempoolTxsMax = mempoolMax
	if mempoolCach != nil {
		v.MempoolCached = *mempoolCach
	}
	if cpu != nil {
		v.CPUPct = *cpu
	}
	v.CPUPctMax = cpuMax
	if mem != nil {
		v.MemPct = *mem
	}
	v.MemPctMax = memMax
	if disk != nil {
		v.DiskPct = *disk
	}
	if netRx != nil {
		v.NetRxBps = *netRx
	}
	if netTx != nil {
		v.NetTxBps = *netTx
	}
	if peerIn != nil {
		v.PeerCountIn = *peerIn
	}
	v.PeerCountInMin = peerInMin
	if peerOut != nil {
		v.PeerCountOut = *peerOut
	}
	v.PeerCountOutMin = peerOutMin
	if behindSentry.Valid {
		tmp := behindSentry.Bool
		v.BehindSentry = &tmp
	}
	if lastObserved != nil {
		v.LastObserved = *lastObserved
	}
	return &v, nil
}
