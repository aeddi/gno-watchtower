# Scribe Event Handlers

Each event kind scribe ingests is implemented by a **handler** under
`internal/scribe/handlers/kinds/`. Per kind: one `.go` file, one `.md` file,
one `_test.go` file. Every handler:

- self-registers in `init()` via `handlers.Register(kind, factory, doc)`
- carries an embedded markdown doc explaining the source signal and payload
- ships with at least one test exercising the `Handle()` path

## Adding a new handler

```bash
make new-handler NAME=validator.something_new
```

This scaffolds `validator_something_new.{go,md,_test.go}` with all required
sections present and TODOs to fill in.

After filling in:

1. Implement `Handle()` and the test.
2. Replace TODOs in the markdown — every section is required by CI.
3. `go test ./internal/scribe/handlers/...`

## Conventions

- `kind` is the event's `Kind` field, e.g. `validator.vote_cast`.
- `Source` indicates the signal type: `log` (Loki), `metric` (VM), or
  `derived` (rule-style — produced from other events).
- `DocRef` is `/docs/handlers/<kind>` and is served from the embedded
  markdown via the scribe HTTP API.
- For event kinds, `Meta` is descriptive; for diagnostics produced by the
  analysis engine, see `internal/scribe/analysis/rules/`.
