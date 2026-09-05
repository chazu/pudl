# Fact and query evidence correctness

**Date:** 2026-09-05

**Tracker:** `pudl-yrn`

**Scope:** assessment item 1, the four reproduced evidence defects

## Outcome

- An ignored fact insert reloads the authoritative stored row before projecting
  current state. Replays preserve temporal bounds, transaction timestamps, and
  provenance through both standalone and transactional writes.
- Migration 17 rebuilds current facts and their search index together, repairing
  legacy replay corruption without rewriting historical rows or IDs. It streams
  live facts through one transaction and records completion in the migration
  ledger. This is a one-time data repair, not a recurring rebuild on every open.
- Derived-query predicates and their bound parameters are built from one ordered
  key list. Multiple constraints cannot independently choose different first
  map entries and thereby omit a predicate.
- Fact identity canonicalizes exact decimal coefficients and exponents instead
  of round-tripping through float64. Nested numbers retain precision, equivalent
  spellings deduplicate, and exponent normalization avoids expanding huge powers.
- Recursive evaluation compiles one variant per derived body occurrence. Each
  variant combines that occurrence's delta with accumulated inputs, including
  another occurrence of the same relation. The next delta is computed before
  merging, using only previously unseen tuples.

## Public interface and compatibility

No public functions or types were added. `Store.AddFact` and `Tx.AddFact` now
return the stored lifecycle on replay and return an error if an existing ID is
used for different identity content. `Store.Query` keeps its existing interface
and returns complete results for the covered nonlinear rules and conjunctions.
The SQL compiler gains an internal per-body-position override for recursive
plans. The existing iteration limit and recursive aggregation rejection remain.

Existing IDs are preserved. Numbers whose decimal values survived the old
floating-point normalization keep their hashes. Previously rounded values can
receive corrected IDs on new insertion; explicit-ID replay preserves an old
reference. A collision with different legacy content is rejected for review.
Already discarded evidence requires recovery from the original source. No
automatic historical rekeying or source reimport is performed.

## Validation

Each of the four public-interface regressions reproduced its assessed failure
before the corresponding fix. The legacy-projection repair and legacy-ID
collision tests also failed before their fixes.

The original assessment program now reports:

```text
retracted replay: Query returned 0, QueryFacts returned 0
two constraints: 0/200 queries returned wrong row count
distinct large integers: same content ID=false, stored fact count=2
nonlinear recursive closure: 6 pairs
```

Passed under `mise exec`:

- `go test ./...`
- `go test -race -gcflags=all=-d=checkptr=0 ./...` (repository CI exception)
- `go vet ./...`
- `go build ./...`
- `go run ./internal/skills/gen -check`

Additional coverage includes transactional replays, historical queries, cyclic
closure, derived inputs from different rounds, nested and negative numbers,
large exponents, legacy numeric-ID compatibility, repeated catalog opens, and
current-fact search-index repair. External mu smoke tests are outside this
fact/query-only slice.
