# Bitemporal Fact Store

The fact store is a general-purpose, bitemporal persistence layer for structured facts. It lives alongside the catalog in the same SQLite database and supports configuration assertions, model dependencies, and Datalog-derived facts.

## Why a Separate Table

The catalog (`catalog_entries`) is purpose-built for imported data artifacts -- files with schemas, content hashes, resource identity, and drift status. Facts are different: they represent typed assertions about the world.

```
"auth package has circular dependency with user package"    -- an observation
"api depends on db"                                         -- a structural fact
"services without integration tests correlate with errors"  -- a derived fact
```

These don't have stored paths, file formats, or record counts. They have a **relation** (what kind of fact), **args** (the specifics), **temporal bounds** (when was it true, when did we learn it), and a **source** (who asserted it).

The catalog and fact store coexist in the same database. The Datalog evaluator (`pudl query`) reads from both -- treating catalog entries as a built-in `catalog_entry` relation (join-only, usable in rule bodies) and facts queried directly by relation name. `catalog_entry` is reserved: you cannot assert facts under that relation. See [datalog.md](datalog.md#catalog-catalog_entry).

## Schema

```sql
CREATE TABLE facts (
    id          TEXT PRIMARY KEY,
    relation    TEXT NOT NULL,
    args        TEXT NOT NULL,       -- JSON object with meaningful keys
    valid_start INTEGER NOT NULL,    -- unix timestamp: when the fact became true
    valid_end   INTEGER,             -- unix timestamp: when it stopped being true (NULL = still true)
    tx_start    INTEGER NOT NULL,    -- unix timestamp: when we recorded this fact
    tx_end      INTEGER,             -- unix timestamp: when this belief ended (retracted or superseded; NULL = current)
    source      TEXT,                -- who asserted this: agent name, "human", "operator", "mu"
    provenance  TEXT,                -- JSON: additional context (agent, activity, etc.)
    supersedes  TEXT,                -- ID of the version an invalidation replaced (migration 19)
    tx_seq      INTEGER,             -- store-wide sequence of the write that recorded this row
    tx_end_seq  INTEGER              -- sequence of the write that ended this belief
);
```

Indexes cover the primary query patterns:

- `idx_facts_relation` -- filter by relation name
- `idx_facts_valid` -- temporal range queries on valid time
- `idx_facts_tx` -- temporal range queries on transaction time
- `idx_facts_tx_seq` -- per-relation write order (history, sequence queries)
- `idx_facts_supersedes` -- following an invalidated fact to its newest version

As query patterns stabilize, SQLite [generated columns](https://www.sqlite.org/gencol.html) can be added to index specific JSON fields within `args` without changing the storage model.

## Bitemporal Model

Every fact has two independent time dimensions:

**Valid time** (`valid_start`, `valid_end`) -- when the fact was true in the real world. A server might have been unhealthy from 2pm to 3pm, regardless of when we learned about it.

**Transaction time** (`tx_start`, `tx_end`) -- when we recorded (and possibly retracted) the fact in the store. This tracks our evolving knowledge. A fact asserted on Monday and retracted (corrected) on Wednesday has `tx_start=Monday, tx_end=Wednesday`.

This gives four query modes:

| Mode | Description | Conditions |
|------|-------------|------------|
| **AsOfNow** | What's currently true and currently asserted | `valid_end IS NULL AND tx_end IS NULL` |
| **AsOfValid(t)** | What was true at time t, per current knowledge | `valid_start <= t AND (valid_end IS NULL OR valid_end > t) AND tx_end IS NULL` |
| **AsOfTransaction(t)** | What we believed at time t | `tx_start <= t AND (tx_end IS NULL OR tx_end > t)` |
| **AsOf(validT, txT)** | What we believed at txT about what was true at validT | Both valid and tx constraints combined |

**AsOfNow** is the common case -- "show me what's true right now." The other modes support post-mortem analysis ("what did we know last Tuesday?") and historical reconstruction ("was this dependency present three months ago?").

### Transaction time is append-only

No operation rewrites what the store believed at an earlier moment. Retraction
only ends a belief (`tx_end`). Invalidation ends the belief in the open version
and records a successor version, in one transaction, that carries the bounded
`valid_end` (see [Retraction vs Invalidation](#retraction-vs-invalidation)). So
an as-of query for a moment before an invalidation still returns the fact with
the unbounded valid time that was believed then.

### Whole seconds and write sequence

Timestamps are whole Unix seconds and belief intervals are half-open,
`[tx_start, tx_end)`. `TxAt = t` means "the state after every write committed
during or before second t." A fact recorded and retracted within the same
second was never part of any whole-second state, so `--as-of-tx` correctly
omits it. It is not lost: it stays in `FactHistory`, and the write sequence
observes it exactly.

Every write allocates the next store-wide sequence number. A row records the
write that created it (`tx_seq`) and the one that ended its belief
(`tx_end_seq`). `FactFilter.TxSeqAt` (`pudl facts list --as-of-tx-seq N`)
selects the state right after write N: `tx_seq <= N AND (tx_end_seq IS NULL OR
tx_end_seq > N)`. It replaces `TxAt`; setting both is an error. Sequences are
monotonic but not dense: an idempotent replay or a failed write consumes a
number. They reflect write order in this store, not caller-supplied
`TxStart` values.

## Operations

### Adding a fact

```go
f, err := db.AddFact(database.Fact{
    Relation: "observation",
    Args:     `{"kind":"obstacle","description":"circular dep in auth","scope":"pudl:pkg/auth"}`,
    Source:   "claude-code",
})
```

If `ValidStart` and `TxStart` are zero, they default to now. The `ID` is computed automatically as SHA256(relation + args + valid_start + source), providing content-addressed deduplication.

Fact ID canonicalization sorts object keys and normalizes decimal number
spellings without converting them through floating point. Distinct integers
such as `9007199254740992` and `9007199254740993` remain distinct; equivalent
spellings such as `1`, `1.0`, and `1e0` deduplicate. This also applies to numbers
inside nested objects and arrays. The original `args` JSON remains stored.

Replaying the same ID returns the original stored fact, including its temporal
bounds, transaction timestamp, and provenance. A replay does not revive a
retracted or invalidated fact. Reusing an ID with different relation, arguments,
valid start, or source returns an error instead of silently discarding evidence.

### Existing-store compatibility

Migration 17 atomically rebuilds `current_facts` and its search index from the
authoritative `facts` table on the next writable catalog open. This repairs stale
rows produced by the old replay behavior. The repair preserves all historical
rows and IDs and runs once, with work proportional to the live fact count.

Existing fact IDs are never renumbered. Number spellings whose values survived
the old floating-point round trip retain their hashes. Previously rounded
numbers can receive corrected IDs on a new insertion; replay an exported fact
with its original ID to preserve an existing reference. A corrected ID that
collides with different legacy content returns an explicit error for review.
Values already discarded by old deduplication cannot be reconstructed from the
catalog; recover those from their original source. Fact identity accepts a
wider numeric domain than the query evaluator. Queries now preserve supported
numeric values and reject unsupported ones explicitly; see the
[numeric query contract](library-api.md#numeric-query-contract). Catalog-item
IDs and the query engine's REAL arithmetic are unchanged.

Migration 19 adds the supersession and write-sequence columns and backfills
sequences for existing rows in recorded time order. Within a single second the
original write order was never recorded, so the backfill places a row's
creation before its closing and, within one second, creations before closings.
Facts that an older pudl invalidated by rewriting `valid_end` in place keep
that row as it is: the belief held before the rewrite was not recorded and
cannot be reconstructed, so as-of-transaction queries over those facts still
reflect the rewritten bound.

### Retraction vs Invalidation

Two distinct operations for two distinct meanings:

**Retract** -- "this record was wrong, we no longer assert it." Sets `tx_end`. The fact disappears from current queries but remains in the audit trail. Use this when correcting a mistake.

```go
err := db.RetractFact(factID)
```

**Invalidate** -- "this was true but isn't anymore." Records a new version of the fact whose `valid_end` is now. The fact is still part of our knowledge (the new version's tx_end stays NULL) but is no longer currently valid. Use this when reality changes.

```go
err := db.InvalidateFact(factID)
latest, err := db.LatestFactVersion(factID) // the version with valid_end set
```

Invalidation does not edit the recorded row. In one transaction it:

1. ends the belief in the open version (`tx_end`, `tx_end_seq`), and
2. inserts a successor with the same relation, args, `valid_start` and source,
   `valid_end` = now, `tx_start` = now, and `supersedes` = the old ID.

The successor's ID is `SHA256("supersedes" + "\x00" + old_id + "\x00" + valid_end)`
(`database.SupersededFactID`); it is not a content address, because the
content matches its predecessor.

Old IDs keep working. `GetFact` returns the exact version an ID names.
`LatestFactVersion` follows the chain to the newest version, and
`FactVersions` returns the whole chain, oldest first. `RetractFact` and
`InvalidateFact` act on the newest version of the ID's fact, so retracting the
old ID of an invalidated fact retracts its successor, and invalidating it again
reports that it is already invalidated. A retracted fact cannot be invalidated.

Example: an agent observes "api depends on db." Later, someone removes that dependency. The original fact gets *invalidated* (a successor with valid_end set to when the dependency was removed), not *retracted* (it was a correct observation at the time).

### Querying

```go
// What observations are currently true?
facts, err := db.QueryFacts(database.FactFilter{
    Relation: "observation",
})

// What did we know last Tuesday?
tuesday := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC).Unix()
facts, err := db.QueryFacts(database.FactFilter{
    Relation: "observation",
    TxAt:     &tuesday,
})

// What was true at deploy time, per our current knowledge?
deployTime := time.Date(2026, 4, 1, 14, 30, 0, 0, time.UTC).Unix()
facts, err := db.QueryFacts(database.FactFilter{
    Relation: "depends",
    ValidAt:  &deployTime,
})
```

### Atomic check-and-write (transactions)

Tools that enforce invariants over the store — "add this preference only if it
doesn't create a cycle", "decide only if no decision stands" — must not check
and write in separate operations: another writer can land between the check
and the write (a TOCTOU race). `WithFactTx` (exposed publicly as
`factstore.Store.Transact`) runs a callback inside one SQLite transaction that
holds the write lock from the start (`BEGIN IMMEDIATE`), so the whole
read–check–write span is serialized against every other writer, in this
process or another:

```go
err := db.WithFactTx(func(tx *database.FactTx) error {
    facts, err := tx.QueryFacts(database.FactFilter{Relation: "dlktk/preference"})
    if err != nil {
        return err
    }
    if wouldCreateCycle(facts, winner, loser) {
        return fmt.Errorf("preference would create a cycle")
    }
    _, err = tx.AddFact(database.Fact{Relation: "dlktk/preference", Args: args})
    return err // non-nil rolls back every write
})
```

The transaction handle offers `AddFact`, `RetractFact`, `InvalidateFact`,
`QueryFacts`, and `FactHistory` with the same semantics as the standalone
operations (current_facts stays in sync). Concurrent transactions block until
the holder commits, bounded by the connection's busy timeout (5s).

## Content-Addressed IDs

Fact IDs are deterministic: `SHA256(relation + "\x00" + canonical_args + "\x00" + valid_start + "\x00" + source)`. This means:

- The same agent recording the same observation at the same time produces the same ID -- natural deduplication.
- Different agents recording the same observation produce different IDs -- corroboration is preserved as signal.
- Args JSON is canonicalized (sorted keys) before hashing, so `{"a":1,"b":2}` and `{"b":2,"a":1}` produce the same ID.

## Args Convention

Args are stored as JSON objects with meaningful keys. There is no enforced schema on args -- different relations use different key structures. Examples:

```json
// observation relation
{"kind": "obstacle", "description": "circular dep in auth", "scope": "pudl:pkg/auth"}

// depends relation
{"from": "api", "to": "db"}

// config relation
{"key": "timeout", "value": "30s", "service": "api"}
```

Use `facts add --schema package.#Definition` when an assertion has an authored
CUE contract. Without `--schema`, relations accept arbitrary JSON objects;
observation and feedback relations have no automatic schema or maturity policy.
The built-in `pudl/dlktk` package remains available to type `dlktk/*` assertions.

## CLI Commands

### `pudl facts add`

Write an assertion under an explicit relation:

```bash
pudl facts add --relation depends --args '{"from":"api","to":"database"}' --source operator
pudl facts add --relation config --args '{"key":"timeout","value":30}' --schema user/config.#Setting
pudl facts add --relation config --args '{"key":"timeout","value":30}' --json
```

`--relation` and `--args` are required; args must be a JSON object. `--source`
defaults to the OS username. Optional `--schema` validates before storing.

### Agent memory retirement

The recall, reflection, curation, promotion, and decay application is removed.
Existing observations, feedback, and maturity fields remain ordinary stored
facts, accessible through current and historical queries and full-text search.
Migration 18 drops only `fact_scored_edb`; it does not rewrite fact bodies, IDs,
temporal bounds, provenance, or the search index. The removed `fact_scored`
relation is no longer a special built-in.

### `pudl facts list`

Query facts from the store:

```bash
# Current observations
pudl facts list --relation observation

# Filter by source
pudl facts list --relation observation --source claude-code

# What was true at deploy time?
pudl facts list --relation depends --as-of-valid 2026-04-01T14:30:00Z

# What did we know last Tuesday?
pudl facts list --relation observation --as-of-tx 2026-03-31T00:00:00Z

# Full details
pudl facts list --relation observation --verbose

# Machine-readable output (an empty result is [])
pudl facts list --relation observation --json

# What did we believe right after write 42?
pudl facts list --relation observation --as-of-tx-seq 42
```

| Flag | Description |
|------|-------------|
| `--relation` | Relation to query (required) |
| `--source` | Filter by source |
| `--as-of-valid` | Query valid time (RFC3339 or Unix timestamp) |
| `--as-of-tx` | Query transaction time (RFC3339 or Unix timestamp) |
| `--as-of-tx-seq` | Query by write sequence instead of `--as-of-tx` |
| `-v, --verbose` | Show full fact details |

### `pudl facts show`

Inspect a single fact by ID. Accepts the full 64-character hex ID or a unique
prefix. The ID of a version superseded by invalidation shows the fact's newest
version; `--exact` shows the version the ID names:

```bash
pudl facts show c0b4392d347a
pudl facts show c0b4392d347a --exact
pudl facts show c0b4392d347a --json
```

### `pudl facts retract`

Mark a fact as retracted -- "we were wrong." Sets `tx_end` so the fact disappears from current queries but remains in the audit trail:

```bash
pudl facts retract c0b4392d347a
```

### `pudl facts invalidate`

Mark a fact as no longer valid -- "reality changed." Records a superseding version with `valid_end` set (and prints its ID), so the fact is no longer current but remains visible in historical queries (`--as-of-valid`, and `--as-of-tx` before the invalidation):

```bash
pudl facts invalidate c0b4392d347a
```

## Connection to the Catalog

The fact store and catalog serve different purposes but live in the same database:

| | Catalog | Fact Store |
|---|---|---|
| **Stores** | Imported data artifacts | Typed assertions |
| **Identity** | Content hash of file | Content hash of fact |
| **Temporal** | import_timestamp, version | Full bitemporal (valid + transaction) |
| **Schema** | CUE schema per entry | JSON args per relation |
| **Use case** | "What data do we have?" | "What do we know?" |

The Datalog evaluator (`pudl query`) treats both as EDB sources -- catalog entries exposed as a `catalog_entry` relation, facts queried directly by relation name. See [datalog.md](datalog.md) for details.
