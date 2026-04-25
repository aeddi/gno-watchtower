-- internal/scribe/store/migrations/001_init.sql
CREATE TABLE IF NOT EXISTS schema_version (
    version    INTEGER PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
    event_id    TEXT PRIMARY KEY,
    cluster_id  TEXT NOT NULL,
    time        TIMESTAMPTZ NOT NULL,
    ingest_time TIMESTAMPTZ NOT NULL,
    kind        TEXT NOT NULL,
    subject     TEXT NOT NULL,
    payload     JSON NOT NULL,
    provenance  JSON NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_cluster_subject_time ON events(cluster_id, subject, time);
CREATE INDEX IF NOT EXISTS idx_events_cluster_kind_time    ON events(cluster_id, kind, time);

CREATE TABLE IF NOT EXISTS samples_validator (
    cluster_id           TEXT NOT NULL,
    validator            TEXT NOT NULL,
    t                    TIMESTAMPTZ NOT NULL,
    tier                 SMALLINT NOT NULL,
    height               BIGINT,
    voting_power         BIGINT,
    catching_up          BOOLEAN,
    mempool_txs          INTEGER,
    mempool_txs_max      INTEGER,
    mempool_cached       INTEGER,
    cpu_pct              REAL,
    cpu_pct_max          REAL,
    mem_pct              REAL,
    mem_pct_max          REAL,
    disk_pct             REAL,
    net_rx_bps           REAL,
    net_tx_bps           REAL,
    peer_count_in        SMALLINT,
    peer_count_in_min    SMALLINT,
    peer_count_out       SMALLINT,
    peer_count_out_min   SMALLINT,
    last_observed        TIMESTAMPTZ,
    PRIMARY KEY (cluster_id, validator, t)
);
CREATE INDEX IF NOT EXISTS idx_sv_tier_t ON samples_validator(tier, t);

CREATE TABLE IF NOT EXISTS samples_chain (
    cluster_id          TEXT NOT NULL,
    t                   TIMESTAMPTZ NOT NULL,
    tier                SMALLINT NOT NULL,
    block_height        BIGINT,
    online_count        SMALLINT,
    online_count_min    SMALLINT,
    catching_up_count   SMALLINT,
    valset_size         SMALLINT,
    total_voting_power  BIGINT,
    PRIMARY KEY (cluster_id, t)
);
CREATE INDEX IF NOT EXISTS idx_sc_tier_t ON samples_chain(tier, t);

CREATE TABLE IF NOT EXISTS state_anchors (
    cluster_id      TEXT NOT NULL,
    subject         TEXT NOT NULL,
    t               TIMESTAMPTZ NOT NULL,
    full_state      JSON NOT NULL,
    events_through  TEXT NOT NULL,
    PRIMARY KEY (cluster_id, subject, t)
);

CREATE TABLE IF NOT EXISTS backfill_jobs (
    id                        TEXT PRIMARY KEY,
    cluster_id                TEXT NOT NULL,
    range_from                TIMESTAMPTZ NOT NULL,
    range_to                  TIMESTAMPTZ NOT NULL,
    chunk_size_seconds        INTEGER NOT NULL,
    started_at                TIMESTAMPTZ NOT NULL,
    last_progress_at          TIMESTAMPTZ NOT NULL,
    last_processed_chunk_end  TIMESTAMPTZ,
    status                    TEXT NOT NULL,
    error_count               INTEGER NOT NULL DEFAULT 0,
    last_error                TEXT
);
