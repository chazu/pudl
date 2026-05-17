# Litelog Adoption Analysis

**Date:** 2026-05-16 (updated from 2026-05-10 draft)  
**Status:** Analysis Complete  
**Question:** Should pudl replace its datalog system with litelog? If not wholesale, what's the integration story?

## Executive Summary

**No to wholesale replacement. Selective integration or parallel use is viable.**

Litelog is a mature Datomic-in-Go: EAV data model, EDN query syntax, Pull API, UDFs, typed attributes, hot/cold partitioning. It's impressive as a standalone datastore. But pudl's datalog serves a different purpose — it's a metadata inference engine over a catalog, not a general-purpose entity store. The impedance mismatch between EAV and pudl's named-relation model makes a full swap costly and philosophically misaligned.

---

## Architecture Comparison (Current State)

| | Pudl (as of 2026-05-16) | Litelog |
|---|---|---|
| **Data model** | Named relations + JSON args | EAV datoms (entity, attribute, value) |
| **Query syntax** | CUE rules + CLI (`pudl query rel field=val`) | EDN Datalog (`[:find ?x :where ...]`) |
| **SQL compilation** | Yes — `internal/datalog/compile.go` | Yes — `query/translate.go` |
| **Recursive eval** | Semi-naive via SQL temp tables | Semi-naive via SQL temp tables |
| **Materialized current state** | `current_facts` table | `current_datoms` table |
| **Bitemporality** | valid_start/end + tx_start/end | valid_from/to + tx |
| **SQLite driver** | `modernc.org/sqlite` (pure Go, `database/sql`) | `zombiezen.com/go/sqlite` (pure Go, direct API) |
| **Identity** | Content-hash SHA256 → proquint | Monotonic entity IDs + TempID |
| **Schema** | CUE-based inference + unification | Attribute registry (type + cardinality) |
| **EDB composition** | FactsEDB + CatalogEDB + MemoryEDB + MultiEDB | Single database only |
| **Pull API** | None | Entity-centric with nested traversal |
| **UDFs** | None | Go functions callable from queries |
| **Rule source** | CUE files (global + repo-scoped, shadowing) | Inline EDN |

---

## What Litelog Has That Pudl Doesn't

### Pull API
Entity-centric data retrieval with nested traversal, wildcards, aliasing. Useful for "give me everything about entity X and its relationships." Pudl's query-only model requires you to know what relations to ask about.

### User-Defined Functions
Register Go functions callable from within Datalog queries. Enables custom predicates (`[(is-stale ?date)]`) without modifying the query compiler.

### Typed Attribute Schema with Cardinality
Explicit CardOne (replaces on write) vs CardMany (accumulates). Pudl's JSON args have no cardinality enforcement at the store level.

### Hot/Cold Partitioning
Separate databases for current state vs history. Keeps working set small regardless of total history.

### Order-Preserving Value Encoding
Binary encoding where byte comparison matches semantic ordering for all types (ints, floats, strings). Enables raw `WHERE v > ?` on BLOBs without function calls.

### Connection Pool with Read/Write Separation
Explicit pool management, per-connection pragma tuning, `ImmediateTransaction` for write serialization.

---

## What Pudl Has That Litelog Doesn't

### Composable EDB Sources
Pudl queries can span multiple data sources in one rule — facts table, catalog entries, in-memory data. This is load-bearing for pudl's use case: deriving relationships between cataloged resources and observed facts.

Litelog queries only its own database.

### Named Relations with Structured Args
`depends(from: "api", to: "auth")` is readable, maps directly to domain concepts, and plays well with CUE validation. EAV equivalent: three separate datoms for entity + from-ref + to-ref + entity-type assertion. More general, but more verbose and harder to reason about for relational data.

### CUE Rule Ecosystem
Rules defined in CUE, loaded from global/repo-scoped paths, validated against CUE schemas. Integrates with pudl's CUE-based schema system. Litelog's EDN rules are standalone — no schema-level integration.

### Content-Addressed Identity
Pudl's SHA256 → proquint IDs are deterministic from content. Same fact from different sources gets same ID. Litelog's monotonic IDs require TempID resolution and can't deduplicate across sources.

### Provenance Tracking
Every fact has optional `source` and `provenance` JSON fields. Litelog tracks transaction metadata but not per-datom provenance.

---

## Integration Options

### Option A: Litelog as Storage Backend (Moderate Disruption)

Replace pudl's `facts`/`current_facts` tables with litelog, mapping pudl's named relations to EAV:

```
pudl: facts.relation="depends", facts.args={"from":"api","to":"auth"}
litelog: [entity1 :depends/from "api"] [entity1 :depends/to "auth"] [entity1 :db/type :depends]
```

**Pros:** Get Pull API, UDFs, typed values, hot/cold for free.  
**Cons:** Impedance mismatch on every query. Lose natural relation semantics. Need adapter layer. Two sqlite drivers in one process. Content-addressed IDs don't map to monotonic entity IDs.

**Verdict:** More work than it saves unless pudl's data model shifts toward entity-centric.

### Option B: Litelog as Parallel Store (Low Disruption)

Keep pudl's existing datalog for catalog inference. Add litelog for a new domain — e.g., rich entity data, user-defined knowledge graphs, or contexts where Pull API and typed schema shine.

**Pros:** Zero disruption to existing functionality. Each store optimized for its domain.  
**Cons:** Two datastores to maintain. Queries can't span both without a bridge layer.

**Verdict:** Viable if pudl grows a use case that's genuinely entity-centric. Premature otherwise.

### Option C: Cherry-Pick Techniques (No Disruption)

Port litelog's best ideas into pudl's existing architecture:

| Technique | Status in Pudl | Value |
|-----------|---------------|-------|
| SQL-compiled queries | **Already done** (`compile.go`) | — |
| Materialized current state | **Already done** (`current_facts`) | — |
| Recursive eval via temp tables | **Already done** (`recursive.go`) | — |
| Pull API equivalent | Not present | Medium — useful for exploration |
| UDFs in queries | Not present | Medium — extensibility |
| Connection pooling | stdlib handles it | Low for CLI tool |
| Hot/cold partitioning | Not present | Low until history grows large |

**Verdict:** The highest-value items from the 2026-05-10 draft (SQL compiler, current_facts, recursive eval) are already implemented. Remaining items are incremental.

### Option D: Adopt Litelog's Query Language (High Disruption)

Replace CUE rules with EDN Datalog syntax. Get litelog's parser/algebrizer/translator as-is.

**Pros:** More expressive query language. Aggregates, not-clauses, or-clauses, parameterized inputs.  
**Cons:** Breaks CUE ecosystem integration. New syntax for users to learn. EDN is Clojure-native, foreign to Go ecosystem.

**Verdict:** Wrong tradeoff. CUE rules integrate with pudl's schema system. EDN doesn't.

---

## Recommendation

**Option C is already mostly complete.** The major architectural wins from litelog's approach (SQL compilation, materialized current state, recursive SQL eval) are implemented in pudl.

Remaining value to consider extracting:

1. **Pull-style entity retrieval** — Add a `pudl pull <entity-id>` command that returns all facts about an entity across relations. Doesn't require EAV; just `SELECT * FROM current_facts WHERE args LIKE '%entity-id%'` with smarter indexing.

2. **UDF support in rules** — Allow Go functions as predicates in CUE rules. Would require extending the rule schema and SQL compiler to emit `SELECT ... WHERE custom_func(arg)`.

3. **Hot/cold partitioning** — When fact history grows large enough to matter, split `facts` into an archive DB. Attach via `ATTACH DATABASE` for temporal queries.

None of these require adopting litelog as a dependency.

---

## Net Assessment

| Question | Answer |
|----------|--------|
| Is litelog a net positive replacement? | **No.** EAV model fights pudl's domain. Integration cost exceeds benefit. |
| Can it integrate without blowing pudl apart? | **Yes (Options B or C).** But Option C's highest-value items are already done. |
| What's left to gain? | Pull API, UDFs, hot/cold — all implementable independently. |
| Should we depend on litelog? | **No.** Different sqlite driver, different data model, different philosophy. Better to borrow ideas than take the dependency. |

---

## Appendix: Driver Compatibility Note

Pudl uses `modernc.org/sqlite` via `database/sql`. Litelog uses `zombiezen.com/go/sqlite` (which wraps modernc internally but exposes a direct connection API). Running both in one process is possible but means two connection management strategies against the same underlying engine. If litelog is ever adopted as Option B, consider forking it to use `database/sql` or accepting the dual-driver complexity.
