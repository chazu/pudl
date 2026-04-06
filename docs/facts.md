# Bitemporal Fact Store

The fact store is a general-purpose, bitemporal persistence layer for structured facts. It lives alongside the catalog in the same SQLite database and serves as the foundation for agent observations, Datalog-derived facts, and eventually the nous reasoning engine.

## Why a Separate Table

The catalog (`catalog_entries`) is purpose-built for imported data artifacts -- files with schemas, content hashes, resource identity, and drift status. Facts are different: they represent typed assertions about the world.

```
"auth package has circular dependency with user package"    -- an observation
"api depends on db"                                         -- a structural fact
"services without integration tests correlate with errors"  -- a derived fact
```

These don't have stored paths, file formats, or record counts. They have a **relation** (what kind of fact), **args** (the specifics), **temporal bounds** (when was it true, when did we learn it), and a **source** (who asserted it).

The catalog and fact store coexist in the same database. Future query infrastructure (the Datalog evaluator) will read from both -- treating catalog entries as one relation among many.

## Schema

```sql
CREATE TABLE facts (
    id          TEXT PRIMARY KEY,
    relation    TEXT NOT NULL,
    args        TEXT NOT NULL,       -- JSON object with meaningful keys
    valid_start INTEGER NOT NULL,    -- unix timestamp: when the fact became true
    valid_end   INTEGER,             -- unix timestamp: when it stopped being true (NULL = still true)
    tx_start    INTEGER NOT NULL,    -- unix timestamp: when we recorded this fact
    tx_end      INTEGER,             -- unix timestamp: when we retracted this record (NULL = current)
    source      TEXT,                -- who asserted this: agent name, "human", "nous", "mu"
    provenance  TEXT                 -- JSON: additional context (agent, activity, etc.)
);
```

Three indexes cover the primary query patterns:

- `idx_facts_relation` -- filter by relation name
- `idx_facts_valid` -- temporal range queries on valid time
- `idx_facts_tx` -- temporal range queries on transaction time

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

## Operations

### Adding a fact

```go
f, err := db.AddFact(database.Fact{
    Relation: "observation",
    Args:     `{"kind":"obstacle","description":"circular dep in auth","scope":["pkg/auth","pkg/user"]}`,
    Source:   "claude-code",
})
```

If `ValidStart` and `TxStart` are zero, they default to now. The `ID` is computed automatically as SHA256(relation + args + valid_start + source), providing content-addressed deduplication.

### Retraction vs Invalidation

Two distinct operations for two distinct meanings:

**Retract** -- "this record was wrong, we no longer assert it." Sets `tx_end`. The fact disappears from current queries but remains in the audit trail. Use this when correcting a mistake.

```go
err := db.RetractFact(factID)
```

**Invalidate** -- "this was true but isn't anymore." Sets `valid_end`. The fact is still part of our knowledge (tx_end stays NULL) but is no longer currently valid. Use this when reality changes.

```go
err := db.InvalidateFact(factID)
```

Example: an agent observes "api depends on db." Later, someone removes that dependency. The original fact gets *invalidated* (valid_end set to when the dependency was removed), not *retracted* (it was a correct observation at the time).

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

## Content-Addressed IDs

Fact IDs are deterministic: `SHA256(relation + "\x00" + canonical_args + "\x00" + valid_start + "\x00" + source)`. This means:

- The same agent recording the same observation at the same time produces the same ID -- natural deduplication.
- Different agents recording the same observation produce different IDs -- corroboration is preserved as signal.
- Args JSON is canonicalized (sorted keys) before hashing, so `{"a":1,"b":2}` and `{"b":2,"a":1}` produce the same ID.

## Args Convention

Args are stored as JSON objects with meaningful keys. There is no enforced schema on args -- different relations use different key structures. Examples:

```json
// observation relation
{"kind": "obstacle", "description": "circular dep in auth", "scope": ["pkg/auth"]}

// depends relation
{"from": "api", "to": "db"}

// config relation
{"key": "timeout", "value": "30s", "service": "api"}
```

As specific relations stabilize, their args structure can be formalized with CUE schemas (e.g., `pudl/nous.#Observation` for the observation relation).

## CLI Commands

### `pudl observe`

Record a structured observation:

```bash
pudl observe "auth package has circular dependency with user package" \
    --kind obstacle \
    --scope pkg/auth,pkg/user

pudl observe "all database calls go through a single connection pool" \
    --kind pattern

pudl observe "the Config struct has 47 fields, should be split" \
    --kind suggestion \
    --scope internal/config \
    --source claude-code
```

Observations are stored as facts in the `observation` relation. The `--kind` flag accepts: fact, obstacle, pattern, antipattern, suggestion, bug, opportunity. The `--source` flag defaults to the current OS user.

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

# Machine-readable output
pudl facts list --relation observation --json
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

The Datalog evaluator (future) will treat both as EDB sources -- catalog entries exposed as a `catalog_entry` relation, facts queried directly by relation name.
