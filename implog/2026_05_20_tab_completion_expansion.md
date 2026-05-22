# Tab Completion Expansion

## Summary
Expanded tab completion coverage from ~13 commands to ~25 commands. Added new completion helpers for relations, sources, fact IDs, and observation kinds. Wired up existing completers to commands that were missing them.

## New Completion Helpers
- `completeRelations` — distinct relation names from current_facts
- `completeSources` — distinct source values from current_facts
- `completeFactIDs` — recent fact hex IDs (12-char prefix) with relation description
- `completeObservationKinds` — static list of known observation kinds

## New DB Methods
- `CatalogDB.GetDistinctRelations()` — SELECT DISTINCT relation FROM current_facts
- `CatalogDB.GetDistinctSources()` — SELECT DISTINCT source FROM current_facts (non-empty)

## Commands Wired Up
- `query` — positional arg (relation) completion
- `facts list` — `--relation`, `--source` completion
- `facts show/retract/invalidate` — positional fact ID completion
- `facts stats` — `--relation` completion
- `pull` — `--kind`, `--source`, `--relation` completion
- `exec` — `--file` (JSON file extension filter)
- `reclassify` — `--ref` (schema names)
- `drift check` — positional arg (definition names)
- `ingest-manifest` — `--origin`
- `ingest-observe` — `--origin`
- `schema add` — positional arg (schema names)
- `schema edit` — positional arg (schema names)
- `export` — `--format` deduped to use shared `completeFormats`

## Tests Added
- `internal/database/facts_distinct_test.go` — unit tests for GetDistinctRelations/GetDistinctSources
- `cmd/completion_test.go` — unit tests for pure completers (formats, sort-by, observation kinds)
