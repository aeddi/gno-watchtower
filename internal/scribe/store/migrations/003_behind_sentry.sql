-- internal/scribe/store/migrations/003_behind_sentry.sql
-- Per-validator "is behind a sentry node" projected state, derived from
-- beacon/sentinel metrics. NULL when the metric isn't yet available.
ALTER TABLE samples_validator ADD COLUMN behind_sentry BOOLEAN;
