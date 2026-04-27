#!/usr/bin/env bash
# Scaffolds a new analysis rule under internal/scribe/analysis/rules/.
# Generates: <slug>.go (boilerplate + init() Register), <slug>.md (doc with TODOs),
# and <slug>_test.go (test skeleton).
#
# Usage: tools/scaffold-rule.sh <slug>
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "usage: $0 <slug>" >&2
  exit 1
fi
slug="$1"
if [[ ! "$slug" =~ ^[a-z][a-z0-9_]*$ ]]; then
  echo "slug must match [a-z][a-z0-9_]*" >&2
  exit 1
fi

dir="internal/scribe/analysis/rules"
go="$dir/$slug.go"
md="$dir/$slug.md"
test="$dir/${slug}_test.go"
struct=$(echo "$slug" | awk -F_ '{for(i=1;i<=NF;i++)$i=toupper(substr($i,1,1))substr($i,2)}1' OFS=)Rule

for f in "$go" "$md" "$test"; do
  if [ -e "$f" ]; then
    echo "$f already exists; refusing to overwrite" >&2
    exit 1
  fi
done

cat >"$go" <<EOF
package rules

import (
	"context"
	_ "embed"

	"github.com/aeddi/gno-watchtower/internal/scribe/analysis"
)

//go:embed ${slug}.md
var ${slug}Doc string

// ${struct} TODO: one-line summary of what this rule detects.
type ${struct} struct {
	// TODO: declare fields the rule needs (e.g., recovery tracker, in-memory window).
}

// Meta returns the rule descriptor used by the engine and /api/rules.
func (r *${struct}) Meta() analysis.Meta {
	return analysis.Meta{
		Code:        "${slug}",
		Version:     1,
		Severity:    analysis.SeverityWarning, // TODO: pick warning|error|critical
		Kinds:       []string{ /* TODO: event kinds to subscribe to, e.g. "validator.*" */ },
		Description: "TODO: one-line description shown in /api/rules.",
		// Params: map[string]analysis.ParamSpec{...} // optional
	}
}

// Evaluate is called by the engine for each Trigger that matches Meta.Kinds
// (event-driven) or each TickPeriod (slow-tick fallback).
func (r *${struct}) Evaluate(ctx context.Context, t analysis.Trigger, d analysis.Deps, emit analysis.Emitter) {
	// TODO: implement detection logic. See spec section 11 for the v1 rule patterns.
	_ = ctx
	_ = t
	_ = d
	_ = emit
}

func init() {
	analysis.Register(&${struct}{}, ${slug}Doc)
}
EOF

cat >"$md" <<EOF
# ${slug}_v1 — TODO: human title

**Severity:** TODO: warning | error | critical
**Gnoland versions:** TODO: vX.Y.Z onward

## What it detects
TODO

## Detection logic
TODO

## Common causes
- TODO

## Remediation / workaround
1. TODO

## Linked signals
- Loki: \`TODO\`
- VictoriaMetrics: \`TODO\`
- Source code: \`TODO\`

## Example incident
TODO or "no example yet"
EOF

cat >"$test" <<EOF
package rules

import (
	"context"
	"testing"
	"time"

	"github.com/aeddi/gno-watchtower/internal/scribe/analysis"
)

func Test${struct}OpensWhenConditionMet(t *testing.T) {
	t.Skip("TODO: implement once Evaluate is filled in")
	r := &${struct}{}
	cfg, _ := analysis.NewRuleConfig(r.Meta(), nil)
	deps := analysis.Deps{ClusterID: "c1", Now: time.Now, Config: cfg}
	var emitted []analysis.Diagnostic
	emit := func(d analysis.Diagnostic) { emitted = append(emitted, d) }
	r.Evaluate(context.Background(), analysis.Trigger{}, deps, emit)
	// TODO: assert emitted.
	_ = emitted
}
EOF

echo "Scaffolded:"
echo "  $go"
echo "  $md"
echo "  $test"
echo ""
echo "Next:"
echo "  1. Fill TODOs in the markdown — every section is required by CI."
echo "  2. Implement Evaluate() in the .go file."
echo "  3. Replace the t.Skip in the test with real assertions."
echo "  4. go test ./internal/scribe/analysis/rules/..."
