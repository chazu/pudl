# SQL Query Compiler: Implementation Plan

**Date:** 2026-05-10  
**Prereqs:** modernc swap (done), current_facts table (done)  
**Estimated effort:** 2-3 days

## Goal

Compile pudl's existing `datalog.Rule` types into parameterized SQL queries that execute against `current_facts` or `facts` (for temporal queries). Replace in-memory semi-naive evaluation for non-recursive rules. Keep recursive evaluation for recursive rules (upgrade to SQL temp tables later).

## Architecture

```
CUE files → datalog.Rule → compiler.Compile() → CompiledQuery{SQL, Params}
                                                       ↓
                                              database.ExecQuery()
                                                       ↓
                                              []datalog.Tuple
```

No new query language. No new AST. Compiler consumes existing `Rule` and `Atom` types.

## Data Model Mapping

Pudl facts are stored as named relations with JSON args:

```
| id | relation     | args                                    |
|----|------------- |-----------------------------------------|
| a1 | observation  | {"target":"svc-a","kind":"healthy"}     |
| a2 | depends      | {"from":"api","to":"svc-a"}             |
```

Each body atom in a rule maps to one self-join on the facts table:

```
Rule:  depends_transitive(from=$A, to=$C) :- depends(from=$A, to=$B), depends(from=$B, to=$C)

SQL:   SELECT json_extract(t0.args, '$.from'), json_extract(t1.args, '$.to')
       FROM current_facts t0, current_facts t1
       WHERE t0.relation = 'depends'
         AND t1.relation = 'depends'
         AND json_extract(t0.args, '$.to') = json_extract(t1.args, '$.from')
```

## Files to Create

### 1. `internal/datalog/compile.go` (~200 lines)

Core compiler. Takes a Rule + options, produces SQL.

```go
type TemporalScope struct {
    ValidAt *int64
    TxAt    *int64
}

type CompiledQuery struct {
    SQL    string
    Params []interface{}
    Head   Atom              // for projecting results back to Tuples
    Vars   map[string]string // variable → SQL expression mapping
}

func Compile(rule Rule, scope TemporalScope) (*CompiledQuery, error)
```

**Compilation steps:**

1. **Table assignment** — each body atom gets alias `t0`, `t1`, etc.
2. **Table selection** — `current_facts` for no temporal scope, `facts` otherwise.
3. **Relation binding** — each alias gets `WHERE tN.relation = ?` predicate.
4. **Ground term binding** — non-variable args become `AND json_extract(tN.args, '$.key') = ?`.
5. **Join inference** — shared `$Variable` across atoms creates equi-join: `json_extract(tN.args, '$.key') = json_extract(tM.args, '$.key2')`.
6. **Temporal injection** — if scope set, add `valid_start <= ? AND (valid_end IS NULL OR valid_end > ?)` etc.
7. **Head projection** — SELECT clause maps head variables to their json_extract expressions.
8. **DISTINCT** — always, since we project a subset of columns.

### 2. `internal/datalog/compile_test.go` (~200 lines)

Test compilation output (SQL text + params) for:
- Single body atom, no joins
- Two body atoms with shared variable (equi-join)
- Ground terms in body (constant filtering)
- Temporal scope injection
- Head with subset of body variables
- Three body atoms (multi-way join)

### 3. `internal/datalog/sql_eval.go` (~150 lines)

SQL-backed evaluator that can replace in-memory eval for non-recursive rules.

```go
type SQLEvaluator struct {
    db    *database.CatalogDB
    rules []Rule
    scope TemporalScope
}

func NewSQLEvaluator(db *database.CatalogDB, rules []Rule, scope TemporalScope) *SQLEvaluator

func (e *SQLEvaluator) Query(relation string, constraints map[string]interface{}) ([]Tuple, error)
```

**Query flow:**
1. Find rules whose head matches `relation`.
2. Compile each matching rule to SQL.
3. Add constraint filters to WHERE clause.
4. Execute SQL against DB.
5. Decode results into `[]Tuple` (json_extract returns native types).
6. If no matching rules, fall back to direct fact lookup (EDB query).

### 4. `internal/datalog/sql_eval_test.go` (~150 lines)

Integration tests with real SQLite:
- Simple fact lookup (no rules needed)
- Single-rule derivation
- Multi-join rule
- Constraint filtering on derived results
- Temporal query mode
- Fallback to EDB for base relations

### 5. Update `cmd/query.go` (~20 line diff)

Wire `SQLEvaluator` as primary evaluator. Fall back to in-memory `Evaluator` for recursive rules.

```go
// Separate recursive from non-recursive rules
recursive, nonRecursive := datalog.PartitionRules(rules)

// Use SQL evaluator for non-recursive
sqlEval := datalog.NewSQLEvaluator(db, nonRecursive, scope)
results, err := sqlEval.Query(relation, constraints)

// If relation not found in SQL results and recursive rules exist, fall back
if len(results) == 0 && len(recursive) > 0 {
    eval := datalog.NewEvaluator(rules, edb)
    results, err = eval.Query(relation, constraints)
}
```

### 6. `internal/datalog/partition.go` (~40 lines)

```go
func PartitionRules(rules []Rule) (recursive []Rule, nonRecursive []Rule)
```

A rule is recursive if any body atom's relation matches any rule's head relation (directly or transitively). Simple check: build set of head relations, flag any rule whose body references that set.

## Implementation Order

```
[DONE] Day 1:
  1. compile.go + compile_test.go
     - Table assignment, relation binding, ground terms
     - Join inference from shared variables
     - Temporal scope injection
     - Head projection

[DONE] Day 2:
  2. sql_eval.go + sql_eval_test.go
     - Wire compiler to database execution
     - Result decoding
     - Constraint filtering
  3. partition.go

[DONE] Day 3:
  4. cmd/query.go update
  5. End-to-end testing with real CUE rules — covered by sql_eval_test.go integration tests
  6. Benchmark: SQL eval vs in-memory eval — deferred to when performance data is needed
```

## Key Design Decisions

**json_extract() for arg access** — Works now, potentially slow at scale. Profile before optimizing. SQLite's json_extract is quite fast for simple key lookups.

**No prepared statement caching (yet)** — Each query compiles and executes fresh. Caching is a future optimization once we see repeated query patterns.

**Keep in-memory evaluator** — Not deleting it. Recursive rules still need it (SQL temp table approach comes later). Non-recursive rules get SQL compilation.

**No CatalogEDB compilation** — The CatalogEDB (catalog_entries as datalog relation) stays in-memory for now. Could compile to SQL later, but catalog_entries has a different schema from facts so it would need separate compilation logic.

**UNION ALL for multiple rules** — If multiple rules derive the same head relation, compile each to SQL and combine with UNION ALL (+ DISTINCT wrapper).

## Example: Full Compilation Trace

### Input Rule (CUE)
```cue
at_risk: {
    head: { rel: "at_risk", args: { service: "$S" } }
    body: [
        { rel: "depends", args: { from: "$S", to: "$D" } },
        { rel: "observation", args: { target: "$D", kind: "unhealthy" } },
    ]
}
```

### Compiled SQL
```sql
SELECT DISTINCT
    json_extract(t0.args, '$.from') AS "service"
FROM current_facts t0, current_facts t1
WHERE t0.relation = ?
  AND t1.relation = ?
  AND json_extract(t0.args, '$.to') = json_extract(t1.args, '$.target')
  AND json_extract(t1.args, '$.kind') = ?
```

### Params
```
["depends", "observation", "unhealthy"]
```

### With Temporal Scope (--as-of-valid)
```sql
SELECT DISTINCT
    json_extract(t0.args, '$.from') AS "service"
FROM facts t0, facts t1
WHERE t0.relation = ?
  AND t1.relation = ?
  AND json_extract(t0.args, '$.to') = json_extract(t1.args, '$.target')
  AND json_extract(t1.args, '$.kind') = ?
  AND t0.valid_start <= ? AND (t0.valid_end IS NULL OR t0.valid_end > ?)
  AND t0.tx_end IS NULL
  AND t1.valid_start <= ? AND (t1.valid_end IS NULL OR t1.valid_end > ?)
  AND t1.tx_end IS NULL
```

## Open Questions

1. **Multi-source joins** — Rules that join facts with catalog_entries (e.g., `depends(from=$S, to=$D), catalog_entry(id=$D, schema=$Schema)`). Defer? Or compile catalog_entry atoms to `catalog_entries` table joins now?

2. **Negation** — Current in-memory evaluator doesn't support negation. Litelog does via NOT EXISTS. Add to SQL compiler? CUE rule syntax would need a `not:` field on atoms.

3. **Aggregates** — Not in current rules. Defer.
