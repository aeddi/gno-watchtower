// Package analysis runs Go-based detection rules over scribe's event stream
// and emits diagnostic events back into the events table. See
// .ignore/docs/superpowers/specs/2026-04-26-scribe-analysis-design.md
// for the full design.
package analysis

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/store"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

// Severity is the operator-facing impact level of a diagnostic.
type Severity string

// Severity levels — see spec §4 for semantics.
const (
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

// State is the lifecycle state of a sustained-condition diagnostic.
type State string

// State values — Open is the default; Recovered pairs with an opening event
// via Diagnostic.Recovers.
const (
	StateOpen      State = "open"
	StateRecovered State = "recovered"
)

// ParamSpec declares a rule-specific configuration parameter and its default.
// Min/Max bounds (when non-nil) are checked at startup.
type ParamSpec struct {
	Default any
	Min     any
	Max     any
}

// Meta describes a rule. Returned by Rule.Meta() and used by the engine and
// /api/rules.
type Meta struct {
	Code        string
	Version     int
	Severity    Severity
	Kinds       []string      // event kinds to subscribe to; supports "x.*" glob
	TickPeriod  time.Duration // 0 = no slow-tick fallback
	Description string
	Params      map[string]ParamSpec
}

// Kind returns the diagnostic event kind this rule emits, e.g.
// "diagnostic.block_missed_v1".
func (m Meta) Kind() string {
	return fmt.Sprintf("diagnostic.%s_v%d", m.Code, m.Version)
}

// Trigger is what the engine hands to Rule.Evaluate. Either Event is non-nil
// (event-driven) or Tick is non-zero (slow-tick fallback).
type Trigger struct {
	Event *types.Event
	Tick  time.Time
}

// Deps carries read-only services rules need.
type Deps struct {
	Store     store.Store
	ClusterID string
	Now       func() time.Time
	Config    RuleConfig
}

// RuleConfig is a typed accessor over the rule's parsed
// [analysis.rules.<code>_v<version>] TOML section. Lookups fall back to the
// rule's declared Meta.Params defaults when the key is unset.
type RuleConfig interface {
	Float64(key string) float64
	Int(key string) int64
	Duration(key string) time.Duration
	String(key string) string
	Bool(key string) bool
}

// Emitter receives diagnostics from a rule and is responsible for routing them
// to the writer. Each rule worker is constructed with its own emitter so the
// rule cannot leak state into another rule's emissions.
type Emitter func(Diagnostic)

// Diagnostic is the rule-level value emitted for one detected pattern. The
// engine wraps it into a types.Event with kind = Meta.Kind() and populates
// the analysis columns.
type Diagnostic struct {
	Subject       string   // "_chain" for chain-level diagnostics
	Severity      Severity // "" = use rule's default (Meta.Severity)
	State         State    // "" defaults to StateOpen
	Recovers      string   // event_id of opening event; required when State=StateRecovered
	Payload       map[string]any
	LinkedSignals []types.SignalLink // resolved Loki / VM queries for this incident
}

// Rule is the contract a rule must satisfy. Rules self-register in init().
type Rule interface {
	Meta() Meta
	Evaluate(ctx context.Context, trigger Trigger, deps Deps, emit Emitter)
}

// KindMatch returns true if evKind matches at least one of the patterns.
// A pattern ending in ".*" matches any kind with that prefix; otherwise it's
// an exact-match check.
func KindMatch(patterns []string, evKind string) bool {
	for _, p := range patterns {
		if strings.HasSuffix(p, ".*") {
			if strings.HasPrefix(evKind, p[:len(p)-1]) { // keep the leading "."
				return true
			}
			continue
		}
		if p == evKind {
			return true
		}
	}
	return false
}
