-- internal/scribe/store/migrations/002_analysis.sql
-- Adds diagnostic-event columns to the events table.
-- severity / state / recovers are NULL for non-diagnostic rows.
ALTER TABLE events ADD COLUMN severity TEXT;
ALTER TABLE events ADD COLUMN state    TEXT;
ALTER TABLE events ADD COLUMN recovers TEXT;

-- DuckDB 1.x does not support partial indexes ("Creating partial indexes
-- is not supported currently"). Workaround: a regular composite index
-- with state near the front so open-diagnostic lookups (state='open' AND
-- kind LIKE 'diagnostic.%') become an index seek and DuckDB filters the
-- kind predicate during the scan.
CREATE INDEX IF NOT EXISTS idx_events_diagnostic_open
    ON events(cluster_id, state, severity, time);
