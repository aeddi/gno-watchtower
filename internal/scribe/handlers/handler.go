// Package handlers contains the per-event-kind handlers that turn raw
// observations into events / samples / anchors. See README.md (added in
// Task 9.5) for how to add a new handler kind.
package handlers

import "github.com/aeddi/gno-watchtower/internal/scribe/normalizer"

// Source enumerates the raw signal type a handler consumes.
type Source string

// Source values.
const (
	SourceLog     Source = "log"
	SourceMetric  Source = "metric"
	SourceDerived Source = "derived"
)

// Meta describes a handler kind. Registered via init() (Task 9.2), surfaced
// by /api/handlers (Task 9.4), and validated by the doc-completeness CI
// guard (Task 9.5).
type Meta struct {
	Kind        string // event kind this handler emits, e.g. "validator.vote_cast"
	Source      Source
	Description string
	DocRef      string // "/docs/handlers/<kind>"
}

// Handler is the interface scribe uses for typed handler dispatch. It embeds
// the normalizer's Handler (Name + Handle) and adds the Meta() descriptor.
type Handler interface {
	normalizer.Handler
	Meta() Meta
}
