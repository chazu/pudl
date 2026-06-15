# cass-memory substrate — Phase D: FTS5 keyword search

Date: 2026-06-14

## Summary

Keyword recall (plan §3.5): `pudl facts search` over the currently-valid facts,
backed by a SQLite FTS5 index kept in sync with current_facts. This is the keyword
half of cass's recall; embeddings/vectors are deliberately out of scope (model
deps belong one layer up).

The FTS5 probe (delete-by-column, MATCH, prefix) was run green before building.

## Design

- Standalone FTS5 table `current_facts_fts(id UNINDEXED, relation UNINDEXED, text)`.
- Indexes the **values** of each fact's args (string/number/bool), not the JSON
  keys — so searching "description" does not match every observation.
- Synced in Go at the two current_facts mutation points (`insertCurrentFact`,
  `deleteCurrentFact`), so it stays consistent across AddFact / RetractFact /
  InvalidateFact and the per-call and transactional write paths. Delete-then-insert
  upsert (FTS5 has no REPLACE). Backfilled from current_facts on first open.
- Search joins current_facts (live filter) and facts (for valid_start/tx_start so
  results carry real timestamps), ordered by FTS5 `rank` (bm25), optional relation
  filter and limit.

## Changes

### `internal/database/facts_fts.go` (new)
- `FactsFTSTable` const; `ensureFactsFTSTable()` (create + backfill);
  `syncFactFTS()` / `deleteFactFTS()` sync helpers; `factSearchText()` (values-only
  extraction); `SearchCurrentFacts(query, relation, limit)` (FTS5 MATCH, ranked).

### `internal/database/current_facts.go`
- `insertCurrentFact` / `deleteCurrentFact` now also sync the FTS index.

### `internal/database/catalog.go`
- `ensureFactsFTSTable()` wired into open, after current_facts is backfilled.

### `cmd/facts_write.go`
- `pudl facts search <query> [--relation R] [--limit N]` (default limit 20).

### `cmd/prime.go`, `cmd/guide.go`
- Documented `pudl facts search`.

## Public surface

```
pudl facts search "<fts5 query>" [--relation R] [--limit N]
```

FTS5 query syntax: bare terms ANDed, "phrases", trailing `*` prefix, AND/OR/NOT.

## Verification

- `internal/database/facts_fts_test.go` (new): term match, AND across terms,
  prefix, JSON-keys-not-indexed, valid_start populated, relation filter, and
  retract-removes-from-index (sync correctness).
- CLI e2e: term / AND / prefix / relation-filter / key-miss / retract-sync all
  correct.
- `CGO_ENABLED=0 go test ./...` full suite green.

## Notes / deferred

- Keyword only; no embeddings/semantic search (out of pudl scope by design).
- The Generator hook combines this (candidate ids by keyword) with `fact_scored`
  (decayed_worth) to rank recall — both keyed by fact id.

Remaining in the plan: the Curator (`pudl facts curate`) and the pith-target
orchestration (ACE cycle + `mu -C`).
