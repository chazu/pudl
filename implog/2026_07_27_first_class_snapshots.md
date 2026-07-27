# First-class observation snapshots (Recommendation 3, D6)

**Date:** 2026-07-27
**Design:** `docs/design/2026-07-27-first-class-snapshots.md`
**Closes:** Recommendation 3 and D6 from `docs/architecture-improvement-report.md`

## What was wrong

A snapshot was a convention: a JSON blob on disk whose identifying fields no
query could reach, stored as a `catalog_entries` row keyed on the hash of its own
bytes. Nothing recorded which model an observation belonged to, so "the current
snapshot for this model" was not a question the catalog could answer. There were
two identifiers for one snapshot — a readable `observe_<timestamp>` inside the
payload that nothing used, and the content hash that everything used. Nothing
pruned, so every observation of every model accumulated forever with one raw file
per record.

## What changed

**An `observe_snapshots` table is the contract.** Everything Recommendation 3
lists is a column: `run_id`, `model`, `workspace`, `origin`, `source`, `targets`,
`record_count`, `created_at`, `retained`. The raw JSON payload stays — it is the
observation's own evidence.

**One identifier, pre-allocated by the run (D6).** `snapshot_id` is the primary
key of `observe_snapshots` *and* the id of the catalog collection entry. The run
session allocates `snap_<proquint>` alongside `run_<proquint>` and hands it to
the ingest. D6 asked for "identifiers for work PUDL initiates are pre-allocated,
and rows are committed on completion"; both halves now hold, and the transaction
delivered by Recommendation 4's first slice is what makes the second true. The
content hash keeps living in `content_hash`, where it belongs.

**Currentness is derived, not stored.** `CurrentObserveSnapshot(model)` is the
newest snapshot of an *observation* source. A `current` column would be a second
source of truth that can disagree with `created_at` — the shape of bug Defect 7
was. Model-instance registrations, which reuse the observe ingester as an
implementation convenience, carry their own source and are excluded: otherwise
every run would shadow its own populate snapshot with the row it writes first.

**Retention exists and is invocable.** `PruneObserveSnapshots` removes snapshots
outside the newest `Keep` for their model **and** older than `OlderThan` — an
AND, so a flag defaulting to zero cannot empty a model's history. Records are
content-addressed and shared between snapshots, so a record goes only when no
remaining snapshot cites it. Raw files are unlinked only under the data dir's
`raw/` tree, and only for entries deleted in the same operation; anything else is
reported rather than silently ignored. Snapshots with no contract row (recorded
before this existed) are never pruned — there is no policy to evaluate against
them. `pudl snapshot list|show|current|retain|prune` is the surface.

**One ingest entry point.** `IngestObserve(db, ObserveIngest{...})` replaces the
three positional `IngestObserveResults*` wrappers, which had reached six
parameters and would have taken ten.

## Bug found and fixed: same-second observations overwrote each other's evidence

The new snapshot-scoping test failed on a pre-existing defect. A record's raw
filename was `<second>_observe_<target>_<index>.json`, a function of (second,
target, position). Two observations of the same target within one second — a
converge loop re-observing, or two models watching one host — wrote both records
to the same path. The catalog entries stayed distinct, so the *first* snapshot
went on pointing at a file that now held the second observation's record, and an
inventory set-diff against that snapshot could report `clean` off data it never
observed.

The filename now carries the content hash instead of the index, making file and
entry one-to-one: two writes can only collide when the content is identical, in
which case the bytes are too — and the dedup path is taken instead. Pinned by
`TestIngestObserve_SameSecondObservationsKeepSeparateEvidence`.

## Public API

- `internal/database`: `ObserveSnapshot`, `PruneOptions`, `PruneResult`, the
  `SnapshotSource*` constants, `RecordObserveSnapshot`, `GetObserveSnapshot`,
  `CurrentObserveSnapshot`, `ListObserveSnapshots`, `RetainObserveSnapshot`,
  `SnapshotRecordEntries`, `PruneObserveSnapshots`. `CatalogWriter` gained
  `RecordObserveSnapshot`. New `observe_snapshots` table; no existing table
  changed.
- `internal/mubridge`: `ObserveIngest`, `ObserveIngestResult`, `IngestObserve`,
  `NewSnapshotID`. The three `IngestObserveResults*` wrappers are gone.
- `internal/acute`: `RunSession.SnapshotID`.
- `cmd`: `pudl snapshot list|show|current|retain|prune`; unexported
  `effectiveWorkspaceName`, `ingestPopulateOutput`, `populateIngest`.
- `internal/database/catalog_rows.go`: `prefixedEntryColumns(alias)` for joins,
  derived from the same `entryColumns` constant.

## Tests

- `internal/database/observe_snapshots_test.go` — contract round-trip; a missing
  contract row is nil rather than an error; current is the newest *for that
  model* and ignores model-instance registrations; list ordering and filtering;
  retention.
- `internal/database/observe_snapshots_prune_test.go` — keep-newest-N; the
  current snapshot is never taken; both conditions required; retention honoured;
  a record still cited by a surviving snapshot survives; raw files unlinked only
  under `raw/` and paths outside reported; legacy snapshots untouched; dry run
  changes nothing but reports real impact; `Keep` is per model.
- `internal/mubridge/observe_snapshot_test.go` — the contract is recorded and
  queryable; the snapshot id *is* the collection entry id; an id is generated
  when no run allocated one; **a failed ingest leaves no snapshot row and no
  collection entry** (D6's actual guarantee); two runs get two snapshots sharing
  their records by membership; the same-second evidence regression.
- `cmd/run_inventory_snapshot_test.go` — a stale snapshot cannot satisfy a newer
  run; another model's snapshot cannot satisfy this one; a snapshot scope is not
  an origin scope; a legacy content-hash scope still resolves.

## Not done here

- No backfill of `observe_snapshots` for pre-existing snapshots: model and
  workspace were never recorded, so a backfill would invent them. Those snapshots
  remain valid replay scopes and are never pruned.
- Drift observations (`drift-observation` entries) are deliberately not folded
  into snapshots. Defect 4 gave them their own entry type so a drift verdict can
  never be mistaken for an observed record; whether they want their own retention
  policy is a separate question.
- No automatic pruning during a run. Retention that deletes without being asked
  is a surprise.
