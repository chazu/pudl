# cass-memory substrate — Phase C: decay-as-a-view

Date: 2026-06-14

## Summary

The recency-weighted recall substrate (plan §3.3): a read-time decay score over
currently-valid facts, exposed to Datalog as the join-only built-in relation
`fact_scored`. Decay is computed at query time and never written back — the
underlying facts are untouched, so bitemporal/historical queries are unaffected.
This keeps pudl a truth store, not a fuzzy ranking engine: the score is a *view*,
not a mutation or a scheduler.

The `pow()` probe (run earlier) confirmed modernc.org/sqlite has the math
functions; the view uses them directly, no fallback needed.

## Model

`fact_scored_edb` joins `current_facts` (the live set) with `facts` (for
`valid_start`) and exposes per fact:

```
id, relation, source,
age_seconds    = unixepoch() - valid_start
worth          = json_extract(args,'$.worth')          (NULL if absent)
decayed_worth  = worth * pow(0.5, age_seconds / 7776000.0)   (90-day half-life)
```

`current_facts` has no timestamp columns (id/relation/args/source/provenance), so
the join to `facts` is required to reach `valid_start`.

## Changes

### `internal/database/builtin_relations.go`
- New constants `FactScoredRelation = "fact_scored"`, `FactScoredView =
  "fact_scored_edb"`. Added `fact_scored` to `reservedRelations` (AddFact rejects
  it; it is join-only).

### `internal/database/fact_scored_view.go` (new)
- `halfLifeSeconds` (90 days) and `ensureFactScoredView()` — drops/recreates the
  view on every open (definition always matches source).

### `internal/database/catalog.go`
- Calls `ensureFactScoredView()` on open, after the facts and current_facts tables
  exist (and after the catalog_entry view).

### `internal/datalog/builtin_edb.go`
- Added `FactScoredRelation → FactScoredView` to `builtinEDBTables`, so body atoms
  referencing `fact_scored` compile to a native-column join (no json_extract /
  temporal filter). The reserved-set/override sync test keeps the two maps aligned.

## Usage

`fact_scored` is join-only (like `catalog_entry`): reference it in a rule body, do
not query it directly. Example — surface playbook facts with their decay score:

```
scored: {
  head: {rel: "scored", args: {id: "$I", w: "$W"}}
  body: [{rel: "fact_scored", args: {id: "$I", relation: "playbook", decayed_worth: "$W"}}]
}
```

## Verification

- `internal/datalog/fact_scored_test.go` (new): a fresh fact scores ~1.0; a
  one-half-life-old fact scores ~0.5; fresh outranks old; direct query rejected
  (join-only); ids round-trip through the view.
- CLI e2e: `pudl query scored` over a fresh playbook fact returns
  `decayed_worth=1.0000, age_seconds=0`; `pudl query fact_scored` is rejected as
  join-only.
- `CGO_ENABLED=0 go test ./...` full suite green.

## Follow-up — RESOLVED (2026-06-14): comparison operators

The threshold gate (`decayed_worth > 0.25`) is now expressible in a rule. See
`implog/2026_06_14_cass_memory_comparison_operators.md`: the Datalog compiler gained
numeric comparison constraints (`>`, `<`, `>=`, `<=`, `!=`) as body-atom terms, so
recency-weighted recall lives in the rule rather than the consumer.

Next: Phase D (FTS5 `facts search`; probe already green), then the Curator
(`pudl facts curate`) and the pith-target orchestration.
