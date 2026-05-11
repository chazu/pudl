# Litelog Adoption Research

**Date:** 2026-05-10  
**Status:** Draft  
**Question:** What should pudl adopt from litelog's bitemporal datalog implementation?

## Context

Pudl has a bitemporal fact store (`internal/database/facts.go`) and an in-memory datalog evaluator (`internal/datalog/eval.go`). Litelog (`~/dev/loosh/dev/litelog`) is a ~14k LOC Datomic-inspired bitemporal datalog-on-SQLite with a query compiler pipeline.

Both are Go. Both use SQLite. Different design philosophies:

| | Pudl | Litelog |
|---|---|---|
| **Data model** | Named relations with typed args (`observation(target, kind, desc)`) | EAV triples (`[entity :attr value]`) |
| **Query language** | Go API + CUE rules | EDN-syntax Datalog compiled to SQL |
| **Evaluation** | In-memory semi-naive (load all facts, iterate in Go) | Datalog → algebrize → SQL (pushdown to SQLite) |
| **Temporal** | valid_start/valid_end + tx_start/tx_end on relation rows | valid_from/valid_to + tx on EAV datoms |
| **SQLite driver** | `mattn/go-sqlite3` (CGo) | `zombiezen.com/go/sqlite` (pure Go) |
| **Identity** | Content-hash IDs (SHA256 → proquint) | Monotonic entity IDs + TempID resolution |
| **Schema** | CUE-based inference | Attribute registry with types + cardinality |

## Recommendation: Cherry-Pick, Don't Wholesale Adopt

Adopting litelog as a dependency would force pudl into EAV triples. Pudl's named-relation model is better for its domain — structured facts with meaningful field names, CUE rule loading, provenance tracking. The EAV model trades readability for generality pudl doesn't need.

Instead, port these specific techniques from litelog into pudl's existing architecture.

---

## 1. SQL-Compiled Query Execution (HIGH VALUE)

### Problem
Pudl's evaluator loads all matching facts into Go memory, then iterates in-process. For small fact stores this works. At scale (thousands of facts, multi-join rules), it creates:
- Memory pressure from materializing entire relations
- No query planning — SQLite's optimizer never sees the join structure
- O(n²) nested-loop joins in Go instead of SQLite's indexed joins

### What Litelog Does
Litelog has a 3-stage compiler pipeline:

```
Datalog AST → Algebrize (resolve schema, infer joins, pick indexes) → SQL text
```

The algebrizer (`query/algebrize.go`) walks WHERE patterns, creates `TableBinding` per pattern, infers equi-join conditions from shared variables, and selects index hints. The translator (`query/translate.go`) emits parameterized SQL with FROM/WHERE/GROUP BY/ORDER BY.

### What Pudl Should Build
A SQL backend for pudl's existing `datalog.Rule` type. Not a new query language — keep CUE rules and the `$Variable` syntax. Add a compilation step:

```
CUE Rules → []datalog.Rule → SQL compiler → parameterized SQL → SQLite execution
```

**Key components to port/adapt:**
- **Join inference from shared variables** — if two body atoms share `$X`, emit `t0.arg_X = t1.arg_X`
- **Table aliasing** — each body atom becomes a self-join on `facts` table with different alias
- **Constant binding** — ground terms become `WHERE arg_foo = ?` predicates
- **Temporal scoping injection** — add valid_start/valid_end/tx_start/tx_end filters based on query mode

**Key difference from litelog:** Pudl facts store args as JSON blob, not as separate columns. Two approaches:
1. **JSON extraction in SQL**: `json_extract(args, '$.target') = ?` — works now, slower
2. **Columnar fact variants**: Add optional typed columns alongside JSON args — future optimization

Start with (1). It gets the join-pushdown benefit immediately. Profile later to decide if (2) is needed.

### Sketch

```go
// internal/datalog/compile.go

type CompiledQuery struct {
    SQL    string
    Params []interface{}
}

func CompileRule(rule Rule, temporal TemporalScope) (*CompiledQuery, error) {
    // Each body atom → aliased self-join on facts table
    // Shared variables → equi-join conditions
    // Ground terms → WHERE predicates
    // Temporal scope → valid/tx time filters
    // Head projection → SELECT clause
}
```

### Effort
Medium. Core compiler ~300 lines. Most complexity is in litelog's algebrizer dealing with EAV column mapping — pudl's named-args model is actually simpler to compile.

---

## 2. Materialized Current-State View (MEDIUM VALUE)

### Problem
Every "what's true now?" query on pudl's facts table requires:
```sql
WHERE valid_end IS NULL AND tx_end IS NULL
```
This scans with index but still filters. As fact volume grows, current-state queries slow down proportionally to total history size.

### What Litelog Does
Maintains a `current_datoms` table — a denormalized cache of only currently-true assertions. Updated transactionally:
- On assert: `INSERT OR REPLACE INTO current_datoms`
- On retract: `DELETE FROM current_datoms`
- On CardOne replace: delete old, insert new

Present-time queries hit `current_datoms` (small, indexed). History queries hit `datoms` (full log).

### What Pudl Should Build
A `current_facts` table mirroring the facts schema minus temporal columns:

```sql
CREATE TABLE IF NOT EXISTS current_facts (
    id          TEXT PRIMARY KEY,
    relation    TEXT NOT NULL,
    args        TEXT NOT NULL,
    source      TEXT,
    provenance  TEXT
);
CREATE INDEX idx_current_facts_relation ON current_facts(relation);
```

Update in `AddFact()` (insert), `RetractFact()` (delete), `InvalidateFact()` (delete). All within same SQLite transaction.

### Effort
Low. ~50 lines of SQL + transaction coordination. Migration is one-time backfill from existing facts.

---

## 3. Recursive Query via SQL Temp Tables (MEDIUM VALUE)

### Problem
Pudl's semi-naive evaluator works but runs entirely in Go memory. For recursive rules (transitive closure, dependency chains), this means materializing the entire closure in-process.

### What Litelog Does
`query/recursive.go` implements semi-naive fixpoint using SQLite temp tables:
1. Create `_rule_X` and `_delta_X` temp tables
2. Populate base case into `_rule_X`
3. Loop: join `_delta_X` with data, insert new rows into `_rule_X`, swap deltas
4. Stop when delta is empty
5. Query `_rule_X` for results
6. Drop temp tables

All iteration happens inside SQLite. Only final results cross the Go/SQLite boundary.

### What Pudl Should Build
Same approach, adapted for pudl's facts table and JSON args. Since pudl already has the semi-naive algorithm in Go, the SQL version follows the same logic — just expressed as SQL statements instead of Go loops.

This depends on (1) — need the SQL compiler first to generate the base/delta SQL.

### Effort
Medium. ~200 lines. Depends on SQL compiler being done first.

---

## 4. Connection Pooling (LOW-MEDIUM VALUE)

### Problem
Pudl uses a single `*sql.DB` from `database/sql`. The standard library pools connections internally, but doesn't expose fine-grained control over WAL mode, pragma tuning per connection, or read/write separation.

### What Litelog Does
Uses `zombiezen.com/go/sqlite`'s `sqlitex.Pool` with:
- Configurable pool size (default 10)
- Per-connection pragma initialization (WAL, synchronous, cache_size, mmap)
- Explicit `ImmediateTransaction` for writes (avoids WAL deadlocks)
- Read connections via `pool.Take()`/`pool.Put()`

### What Pudl Should Do
This ties into the SQLite driver question (Section 7 below). If pudl stays on `database/sql`, connection pooling is handled by the stdlib. If pudl moves to zombiezen, explicit pool management becomes available and worthwhile.

Current value: low. Pudl is single-user CLI tool. Pool matters more for concurrent access (agents, servers).

---

## 5. Order-Preserving Value Encoding (LOW VALUE FOR NOW)

### Problem
Not a current problem. Pudl stores args as JSON text. Comparisons work via `json_extract()` which handles type coercion.

### What Litelog Does
Order-preserving binary encoding for all value types. Enables raw BLOB comparison in SQL without function calls. Floats use IEEE 754 bit manipulation so byte-order matches numeric order.

### Assessment
Clever but only matters for EAV stores where values share a single column. Pudl's JSON args model doesn't benefit — `json_extract()` already returns properly typed values for comparison. File this under "interesting technique, not needed now."

Would become relevant if pudl ever adopts columnar fact storage (Section 1 option 2).

---

## 6. Index Hint Selection (LOW VALUE FOR NOW)

### What Litelog Does
Algebrizer picks index hints (EAVT, AEVT, AVET, VAET) based on which pattern elements are bound. Emits `INDEXED BY` in SQL.

### Assessment
Relevant for EAV stores with multiple covering indexes. Pudl's facts table has simpler indexing (relation, valid time, tx time). SQLite's query planner handles these well without hints. Revisit if profiling shows bad query plans.

---

## 7. SQLite Driver: mattn vs zombiezen vs modernc

This is the second major decision. Current state:

| Driver | Type | API | Used By |
|---|---|---|---|
| `mattn/go-sqlite3` | CGo wrapper | `database/sql` | **pudl** |
| `zombiezen.com/go/sqlite` | Pure Go wrapper | Direct `*sqlite.Conn` | **litelog** |
| `modernc.org/sqlite` | Pure Go engine | `database/sql` | (underlying engine for zombiezen) |

### CGo vs Pure Go

**mattn/go-sqlite3 (CGo):**
- Wraps system/bundled C SQLite via CGo
- Requires C compiler for builds
- Cross-compilation is painful (need cross-compiler toolchain)
- Slightly faster for heavy workloads (C SQLite is heavily optimized)
- `database/sql` interface — familiar, well-documented
- Cannot use `CGO_ENABLED=0`

**modernc.org/sqlite (Pure Go):**
- Machine-translated C → Go. No CGo, no C compiler needed
- `CGO_ENABLED=0` works — easy cross-compilation
- `database/sql` compatible (drop-in for mattn in most cases)
- ~10-20% slower than C SQLite in benchmarks
- Larger binary size (~30MB more)

**zombiezen.com/go/sqlite (Pure Go wrapper around modernc):**
- Thin wrapper over modernc with better ergonomics
- Direct `*sqlite.Conn` API — no `database/sql` overhead
- Explicit connection management (pool, take, put)
- Prepared statement caching built in
- Blob I/O support
- Same pure-Go benefits as modernc

### Recommendation: Move to modernc, Consider zombiezen Later

**Phase 1: mattn → modernc (drop-in replacement)**

Minimal change. Replace import:
```go
// Before
_ "github.com/mattn/go-sqlite3"

// After  
_ "modernc.org/sqlite"
```

Pudl uses only standard `database/sql` interface. This swap should be nearly transparent. Benefits:
- No CGo dependency — easier builds, easier cross-compilation
- `CGO_ENABLED=0` support
- Same API, same behavior

**Phase 2: Evaluate zombiezen for the SQL compiler layer**

When building the SQL-compiled query execution (Section 1), consider using zombiezen's direct API for that specific code path. Benefits:
- Prepared statement objects with explicit lifecycle
- Per-connection pragma control
- Pool-based read/write separation
- Better fit for "compile query, execute prepared statement" pattern

This doesn't mean converting all existing `database/sql` code. The SQL compiler could use zombiezen internally while catalog operations stay on `database/sql`. Or, if the migration goes smoothly, convert everything.

**Phase 3: Full zombiezen (optional, only if Phase 2 proves worthwhile)**

Convert remaining `database/sql` usage. Larger effort, only justified if Phase 2 shows clear benefits.

### Risk Assessment

- **modernc swap (Phase 1):** Very low risk. Same `database/sql` interface. Run test suite to verify.
- **zombiezen adoption (Phase 2):** Medium risk. Different API, new dependency, mixed driver situation during transition. But contained to new code.
- **Full zombiezen (Phase 3):** Higher effort but still mechanical. Every `db.Query/Exec/Scan` call changes shape.

---

## Implementation Order

```
1. modernc swap (Phase 1)           — 1 hour, low risk
2. current_facts materialized view  — 1 day, low risk  
3. SQL query compiler               — 2-3 days, medium complexity
4. Recursive query via temp tables  — 1-2 days, depends on (3)
5. zombiezen for compiler (optional) — 1-2 days, evaluate after (3)
```

Steps 1 and 2 are independent and can be done in any order. Steps 3 and 4 are sequential. Step 5 is opportunistic.

## What NOT to Adopt

- **EAV data model** — pudl's named relations are better for its domain
- **EDN query syntax** — CUE rules are pudl's existing convention and integrate with schema system
- **Attribute interning** — only needed for EAV stores
- **TempID resolution** — pudl uses content-addressed IDs, which are better for its append-only, multi-source model
- **Order-preserving BLOB encoding** — no benefit with JSON args model
- **INDEXED BY hints** — premature; SQLite planner is good enough for pudl's index structure

## Open Questions

1. **JSON extraction performance** — how fast is `json_extract()` in WHERE clauses at scale? Need to benchmark before committing to JSON-in-SQL approach for the compiler.
2. **Mixed driver feasibility** — can `database/sql` and zombiezen coexist on same DB file? (Yes, but need separate connection management.)
3. **Rule complexity ceiling** — how complex do pudl's datalog rules actually get? If rules stay simple (2-3 body atoms, no deep recursion), the SQL compiler may be over-engineering. Profile the in-memory evaluator first.
