# Fact store: append-only invalidation and write sequence

**Date:** 2026-10-05

## Problem

`InvalidateFact` ran `UPDATE facts SET valid_end = ?` on the recorded row,
rewriting what the store had believed in the past. Reproduced: add a fact,
invalidate it later, then ask `--as-of-valid LATER --as-of-tx T1` for a moment
T1 before the invalidation. The correct answer is the fact (at T1 nothing said
it would end); the store returned `null`. Separately, whole-second timestamps
could not order two writes in the same second, and a fact added and retracted
within one second could not be observed by any as-of query.

## What changed

### Append-only invalidation (plan D)

- Invalidation now, in one transaction, ends the belief in the open version
  (`tx_end`, `tx_end_seq`) and inserts a successor version: same relation,
  args, `valid_start`, source and provenance; `valid_end` = `tx_start` = now;
  `supersedes` = the old ID. Its ID is `SupersededFactID(old, valid_end)`,
  not a content address (the content equals its predecessor's).
- `GetFact` still returns the exact recorded version. New
  `LatestFactVersion(id)` follows the chain forward; `FactVersions(id)` returns
  the whole chain oldest first. `RetractFact` and `InvalidateFact` resolve an
  ID to its newest version, so old IDs keep working for writes.
- Invalidating an already-invalidated or retracted fact returns a NotFound
  error. Invalidating a retracted fact previously succeeded by setting
  `valid_end` on the retracted row.
- Datalog temporal filters need no change: at any transaction time exactly one
  of the two versions is believed.

### Write sequence

- Every fact write first allocates the next number from a one-row
  `fact_sequence` counter (`UPDATE … RETURNING`). That statement also takes the
  write lock, so a deferred transaction never upgrades from a read snapshot.
- Rows record `tx_seq` (the write that created them) and `tx_end_seq` (the
  write that ended their belief). `FactHistory` orders by `tx_seq`;
  `QueryFacts` breaks `valid_start` ties by `tx_seq`.
- `FactFilter.TxSeqAt` / `pudl facts list --as-of-tx-seq N` selects the state
  right after write N. It is mutually exclusive with `TxAt`.

### Same-second semantics (decision)

`TxAt = t` keeps its meaning: the state after every write committed during or
before second t, with half-open belief intervals `[tx_start, tx_end)`. A fact
added and retracted within one second was never part of any whole-second
state, so whole-second queries correctly omit it. Closing the interval at both
ends would make "retracted at t" and "believed at t" true at once. The fact
stays in `FactHistory`, and `TxSeqAt` observes it exactly. This is documented
in `docs/facts.md#whole-seconds-and-write-sequence`.

### Migration 19 `fact_versioning`

Adds `supersedes`, `tx_seq`, `tx_end_seq`, the `idx_facts_supersedes` (partial)
and `idx_facts_tx_seq` indexes, and `fact_sequence`. It backfills sequences
for existing rows in recorded time order (within one second, creations before
closings), then seeds the counter after the highest sequence. It is idempotent:
it skips existing columns, only sequences rows that lack one, and seeds the
counter with `MAX`. **Not repairable:** rows an older pudl invalidated in place
keep their rewritten `valid_end`, because the belief they held before the
rewrite was never recorded.

### CLI

- `pudl facts list --json` prints `[]` for an empty result. `scanFactRows`
  returns a non-nil slice, and the `--source` filter starts non-nil.
- `pudl facts show <id>` shows the fact's newest version, noting when the given
  ID was superseded; `--exact` shows the version the ID names. Verbose output
  shows `Seq` and `Supersedes`, and labels a closed belief "no longer believed"
  (retracted or superseded) rather than "retracted".
- `pudl facts invalidate` prints the new version's ID.

## Public API

- `database.Fact`: new fields `TxSeq int64`, `TxEndSeq *int64`,
  `Supersedes string` (store-assigned; values supplied to `AddFact` are
  ignored).
- `database.FactFilter.TxSeqAt *int64`.
- `database.SupersededFactID(predecessorID string, validEnd int64) string`.
- `(*CatalogDB).LatestFactVersion(id string) (*Fact, error)`,
  `(*CatalogDB).FactVersions(id string) ([]Fact, error)`.
- `factstore.Store.LatestFactVersion`, `factstore.Store.FactVersions`.
  `factstore.Fact`/`FactFilter` gain the fields above through their aliases.

## Tests

`internal/database/fact_versions_test.go`:

- the reproduced as-of case;
- invalidation × `TxAt` before and after;
- same-second add + retract (absent at whole seconds, present by sequence,
  kept in history);
- `TxAt` together with `TxSeqAt` rejected;
- old IDs resolving through the chain for reads and writes;
- invalidating a retracted fact fails;
- monotonic sequences that ignore caller-supplied values;
- the migration backfilling a legacy-shaped catalog.

Existing tests that asserted in-place invalidation now assert the two-version
shape (`facts_test.go`, `facts_tx_test.go`, `pkg/factstore/replay_test.go`).
