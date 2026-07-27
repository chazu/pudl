# Design: first-class observation snapshots

**Date:** 2026-07-27
**Implements:** Recommendation 3, and D6, from `docs/architecture-improvement-report.md`
**Status:** stabilized after adversarial review (§8); ready to execute

## 1. What a snapshot is today

A convention, not an object. `createObserveSnapshot` marshals a map —
`snapshot_id`, `timestamp`, `origin`, `targets`, `record_count`,
`schema_summary`, optionally `run_id` — writes it to a raw file, hashes the
bytes, and stores a `catalog_entries` row whose `id` **is that content hash**,
with `collection_type='collection'` and `entry_type='observe'`. Members are
linked by `collection_memberships`.

Three consequences:

- **The contract is unreadable from SQL.** Everything that identifies the
  snapshot lives inside a JSON blob on disk. No query can ask "which snapshots
  belong to this model", because the model is not recorded anywhere at all.
- **There are two identifiers.** The `snapshot_id` inside the payload
  (`observe_20260727_101530.000000000`) is not the ID anything uses; callers and
  `--catalog-scope` use the content hash. The readable one is decorative.
- **Nothing prunes.** Every observation of every model accumulates forever,
  along with one raw file per record.

## 2. Decisions

### 2.1 An `observe_snapshots` table is the contract

```
snapshot_id   TEXT PRIMARY KEY   -- also the catalog collection entry id
run_id        TEXT NOT NULL DEFAULT ''
model         TEXT NOT NULL DEFAULT ''
workspace     TEXT NOT NULL DEFAULT ''   -- effective origin: workspace name or "global"
origin        TEXT NOT NULL DEFAULT ''   -- ingest origin
source        TEXT NOT NULL DEFAULT ''   -- mu-observe | ewe | ingest-observe
targets       TEXT NOT NULL DEFAULT '[]' -- JSON array
record_count  INTEGER NOT NULL DEFAULT 0
created_at    TIMESTAMP NOT NULL
retained      INTEGER NOT NULL DEFAULT 0 -- pinned: never pruned
```

Everything Recommendation 3 lists — workspace, model, target, run ID, source,
creation time, retention — becomes a column. The raw JSON payload stays: it is
the observation's own evidence, and nothing here replaces it.

### 2.2 One identifier, pre-allocated by the run (D6)

`snapshot_id` is the primary key of `observe_snapshots` **and** the `id` of the
catalog collection entry. The content hash stops being an identifier and
becomes what it always was — a content hash, kept in `content_hash`.

D6 asked for "identifiers for work PUDL initiates are pre-allocated, and rows
are committed on completion". The run session now allocates
`snap_<proquint>` alongside `run_<proquint>` and hands it to the ingest, which
writes the row inside the transaction that writes the members. So:

- a partially-ingested observation leaves **no** snapshot row (the transaction
  from Recommendation 4's first slice already guarantees this), and
- the run knows the ID it *would* have had, so a failure can name it.

Snapshot dedup stays rejected (Defect 11): two observations are two snapshots.

**Backwards compatibility.** Existing snapshots keyed by content hash keep
working: `--catalog-scope <hash>` resolves through the same
`GetCollectionByID` path, and a snapshot with no `observe_snapshots` row is
still a valid scope. Only the *new* metadata is unavailable for them, which is
what a backfill cannot invent.

### 2.3 Currentness is derived, not stored

The current snapshot for a model is the newest one. That is a query, not a
column. Storing a `current` flag would create a second source of truth that can
disagree with `created_at` — the exact failure Defect 7 was, one table over.

```go
func (c *CatalogDB) CurrentObserveSnapshot(model string) (*ObserveSnapshot, error)
```

### 2.4 Snapshot-scoped queries go through memberships

`SnapshotRecordEntries(snapshotID)` reads the snapshot's members via
`collection_memberships` rather than the legacy `collection_id` column. This is
the first consumer written against memberships-as-sole-source, which
Recommendation 4 wants to make universal; the legacy column stays until that
lands.

Inventory drift keeps its current shape (`--catalog-scope` → collection ID or
origin) and gains nothing new, because it is already correctly scoped after
Defect 3. What it gains is *tests* proving the scoping holds — Recommendation 3
asks for them and none exist.

### 2.5 Retention

```go
type PruneOptions struct {
    Model     string     // empty: every model
    Keep      int        // newest N per model are always kept
    OlderThan time.Time  // zero: no age condition
    DryRun    bool
}
func (c *CatalogDB) PruneObserveSnapshots(opts PruneOptions) (PruneResult, error)
func (c *CatalogDB) RetainObserveSnapshot(snapshotID string, retained bool) error
```

A snapshot is pruned when it is **not** among the newest `Keep` for its model,
**and** older than `OlderThan` when set, **and** not `retained`. Both conditions
must hold, so a `Keep` of 0 does not wipe a model's history unless an age cutoff
also says so, and an age cutoff cannot delete the only snapshot a model has.

Pruning removes, in one transaction: the snapshot row, its memberships, the
collection entry, and any **observe item** entry left with zero memberships.
Items are content-addressed and shared between snapshots, so the membership
count is what decides — deleting an item still cited by another snapshot would
silently empty that snapshot instead.

Raw files are unlinked for the entries actually deleted, and only when the path
lies under the data dir's `raw/` tree. Without that, pruning reclaims no disk at
all, which is most of the point; with the prefix check, a bug in path handling
cannot unlink anything outside pudl's own staging area.

Snapshots with no `observe_snapshots` row — pre-existing ones — are **never**
pruned. They carry no model and no retention flag, so no policy can be evaluated
against them, and deleting what cannot be evaluated is not a policy.

### 2.6 CLI

```
pudl snapshot list [--model <name>] [--limit N]
pudl snapshot show <id>
pudl snapshot current <model>
pudl snapshot retain <id> [--release]
pudl snapshot prune [--model <name>] [--keep N] [--older-than <dur>] [--dry-run]
```

Retention that cannot be invoked is not retention.

### 2.7 The ingest signature

`IngestObserveResultsWithSnapshotRunID(db, reader, origin, dataDir, graph, runID)`
gains a model, a workspace, a source and a pre-allocated ID; six positional
parameters becoming ten is not acceptable. One request struct replaces them:

```go
type ObserveIngest struct {
    Reader     io.Reader
    DataDir    string
    Graph      *inference.InheritanceGraph
    SnapshotID string   // pre-allocated; generated when empty
    RunID      string
    Model      string
    Workspace  string
    Origin     string   // defaults to "mu-observe"
    Source     string   // defaults to "ingest-observe"
}
func IngestObserve(db *database.CatalogDB, in ObserveIngest) (ObserveIngestResult, error)
```

The three existing exported wrappers collapse into it. They are all reachable
only from `cmd` and `internal`, so this is not a public API break.

## 3. Blast radius

| Surface | Change |
|---|---|
| `internal/database` | new `observe_snapshots.go`; no change to existing tables |
| `internal/mubridge` | `IngestObserve` replaces three wrappers; snapshot entry keyed by snapshot ID |
| `internal/acute` | `RunSession` allocates `SnapshotID` |
| `cmd` | `snapshot` command; populate/drift pass the pre-allocated ID |
| `pkg/` | none |

## 4. Tests

| Case | Assertion |
|---|---|
| Ingest records the contract | model, workspace, source, targets, run ID, count all queryable |
| Snapshot ID is the collection entry ID | one identifier, not two |
| Pre-allocated ID is honoured | the run's ID is what lands |
| Failed ingest | no snapshot row and no collection entry |
| Current snapshot | newest for that model; unaffected by other models' snapshots |
| Snapshot-scoped records | returns exactly that snapshot's members |
| Inventory drift vs a stale snapshot | drift computed against the named snapshot only |
| Inventory drift vs another model's snapshot | cannot satisfy this model's desired records |
| Prune keeps the newest N | and never the current one |
| Prune respects `retained` | pinned snapshots survive |
| Prune requires both conditions | `Keep` alone with no age cutoff, and vice versa |
| Prune and shared items | an item still in another snapshot survives |
| Prune unlinks raw files | only under `raw/` |
| Prune ignores legacy snapshots | rows without an `observe_snapshots` entry are untouched |
| Legacy content-hash scope | still resolves |

## 5. Sequencing

Depends on the catalog transaction (delivered). Feeds Recommendation 4's
retirement of the legacy collection columns by being the first consumer written
against memberships only.

## 6. Non-goals

- No change to inventory drift's scope resolution. Defect 3 fixed it; this adds
  the tests that were missing, not a second mechanism.
- No backfill of `observe_snapshots` for existing snapshots. Model and workspace
  were never recorded, so a backfill would be inventing them.
- No automatic pruning on run. Retention that deletes without being asked is a
  surprise; the command is explicit and `--dry-run`-able.

## 7. Adversarial review

**A1 — "Changing the snapshot entry's `id` from the content hash to a
pre-allocated ID breaks every existing `--catalog-scope`."** *Checked, it does
not.* Scope resolution is `GetCollectionByID(scope)`, keyed on
`catalog_entries.id`. Existing rows keep their content-hash ids and keep
resolving; new rows resolve by their new ids. Nothing rewrites an existing row.
The one real loss is that a script hard-coding a *future* snapshot's hash would
break — but hashes are not predictable, so no such script exists. Covered by a
regression test.

**A2 — "Deleting item entries during prune will silently empty other
snapshots."** *Landed.* Items are content-addressed and deliberately shared
across snapshots — that is what `ingestObserveRecord`'s dedup does. The rule is
therefore membership-count-driven (§2.5), not "delete this snapshot's items".

**A3 — "Unlinking raw files during prune is dangerous."** *Partially landed.* It
is, and pruning that reclaims nothing is also useless. Resolution: unlink only
paths under the data dir's `raw/` prefix, and only for entries actually deleted
in the same operation. A path outside that tree is left alone and reported.

**A4 — "`Keep` and `OlderThan` as an AND makes `--keep 0` a no-op, which will
surprise someone who wants to wipe a model."** *Accepted deliberately.* The
surprising direction is the safe one. A prune that can empty a model's history
because a flag defaulted to zero is the more expensive surprise, and the
explicit form (`--keep 0 --older-than 0s`) is still available.

**A5 — "Storing `workspace` on the snapshot duplicates state that
`internal/workspace` owns."** *Rejected.* It records where the observation *came
from*, which is history, not configuration. Reading it back from the current
workspace context would answer a different question — where the reader is
standing — and would silently relabel snapshots when a repo moves.

**A6 — "`CurrentObserveSnapshot` by newest `created_at` is ambiguous for two
snapshots in the same instant."** *Landed as an ordering detail.* Ties break on
`rowid DESC`, so insertion order decides, matching `AppliesSinceLastClean`.

**A7 — "This does not make snapshots first-class for the *drift* path, which
records `drift-observation` entries with no snapshot at all."** *Accepted as
out of scope, explicitly.* Defect 4 deliberately gave drift observations their
own entry type so a drift verdict can never be mistaken for an observed record
by inventory drift. Folding them into snapshots would undo that distinction.
What a drift observation is, and whether it deserves its own retention policy,
is a separate question this design does not answer — recorded here rather than
left implicit.

**A8 — "A pre-allocated snapshot ID is allocated even for runs that never
observe."** *True and harmless.* It is a string in memory; nothing is written
until an ingest commits. The alternative — allocating lazily at the ingest —
is what D6 rejected, because then the run cannot name the snapshot a failed
ingest would have produced.
