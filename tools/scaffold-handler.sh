#!/usr/bin/env bash
# Scaffolds a new event handler under internal/scribe/handlers/kinds/.
# Generates: <slug>.go (boilerplate + init() Register), <slug>.md (doc with TODOs),
# and <slug>_test.go (test skeleton).
#
# Usage: tools/scaffold-handler.sh <event_kind>
#   e.g.: tools/scaffold-handler.sh validator.something_new
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "usage: $0 <event_kind>" >&2
  exit 1
fi
kind="$1"
slug=$(echo "$kind" | tr '.' '_')
struct=$(echo "$slug" | awk -F_ '{for(i=1;i<=NF;i++)$i=toupper(substr($i,1,1))substr($i,2)}1' OFS=)

dir="internal/scribe/handlers/kinds"
go="$dir/$slug.go"
md="$dir/$slug.md"
test="$dir/${slug}_test.go"

for f in "$go" "$md" "$test"; do
  if [ -e "$f" ]; then
    echo "$f already exists; refusing to overwrite" >&2
    exit 1
  fi
done

cat >"$go" <<EOF
package kinds

import (
	"context"
	_ "embed"

	"github.com/aeddi/gno-watchtower/internal/scribe/handlers"
	"github.com/aeddi/gno-watchtower/internal/scribe/normalizer"
	"github.com/aeddi/gno-watchtower/internal/scribe/types"
)

//go:embed ${slug}.md
var ${slug}Doc string

// ${struct} TODO: one-line summary of what this handler emits.
type ${struct} struct{ cluster string }

// New${struct} returns a fresh ${struct} bound to the given cluster.
func New${struct}(cluster string) *${struct} { return &${struct}{cluster: cluster} }

// Name returns the handler's diagnostic name (used for log breadcrumbs).
func (${struct}) Name() string { return "${slug}" }

// Meta returns the handler descriptor used by the registry, /api/handlers, and CI.
func (${struct}) Meta() handlers.Meta {
	return handlers.Meta{
		Kind:        "${kind}",
		Source:      handlers.SourceLog, // TODO: SourceLog | SourceMetric | SourceDerived
		Description: "TODO",
		DocRef:      "/docs/handlers/${kind}",
	}
}

// Handle inspects the observation and returns 0+ ops.
func (h *${struct}) Handle(ctx context.Context, o normalizer.Observation) []types.Op {
	// TODO: detection logic, return ops.
	_ = ctx
	_ = o
	return nil
}

func init() {
	handlers.Register("${kind}",
		func(cluster string) handlers.Handler { return New${struct}(cluster) },
		${slug}Doc)
}
EOF

cat >"$md" <<EOF
# ${kind} — TODO: human title

**Source signal type:** TODO: log | metric | derived
**Gnoland versions:** TODO: vX.Y.Z onward

## What it represents
TODO

## How it's detected
TODO

## Linked source code
- gnoland: \`TODO\`

## Payload fields
- \`field_name\` (type) — TODO
EOF

cat >"$test" <<EOF
package kinds_test

import "testing"

func Test${struct}EmitsExpectedEvent(t *testing.T) {
	t.Skip("TODO: implement once Handle is filled in")
}
EOF

echo "Scaffolded:"
echo "  $go"
echo "  $md"
echo "  $test"
echo ""
echo "Next:"
echo "  1. Fill TODOs in the markdown — every section is required by CI."
echo "  2. Implement Handle() in the .go file."
echo "  3. Replace the t.Skip with real assertions."
echo "  4. go test ./internal/scribe/handlers/..."
