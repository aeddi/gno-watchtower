package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"

	"github.com/aeddi/gno-watchtower/internal/scribe/store/migrations"
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
