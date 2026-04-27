// Package rules registers the v1 scribe analysis rules. Blank-imported by
// cmd/scribe/run.go so each rule's init() runs and registers itself with
// internal/scribe/analysis.
//
// Phase 7 of the implementation plan adds the rule files (block_missed,
// bft_at_risk, consensus_slow, consensus_stuck, validator_isolated). Until
// then this package is intentionally empty — blank-importing it costs nothing.
package rules
