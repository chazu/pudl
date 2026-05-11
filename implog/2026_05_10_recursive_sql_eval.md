# Recursive Datalog Evaluation via SQL Temp Tables

**Date:** 2026-05-10

## Summary

Implemented recursive datalog evaluation using SQLite temp tables, replacing the in-memory semi-naive evaluator for recursive rules. All recursive rule evaluation now happens inside SQLite, with only final results crossing the Go/SQLite boundary.

## What Was Built

### `internal/datalog/recursive.go` (~190 lines)

`EvalRecursive(db, rules, relation, constraints, scope)` implements semi-naive fixpoint evaluation using three temp tables per derived relation:

- `_rule_<rel>` -- accumulates all derived facts (with PK dedup)
- `_delta_<rel>` -- holds facts from previous iteration for semi-naive joins
- `_new_<rel>` -- staging area for current iteration's derived facts

Algorithm:
1. Partition rules into base (non-recursive) and recursive
2. Create temp tables for each derived relation
3. Seed base case by compiling base rules to SQL and inserting results
4. Fixpoint loop: compile recursive rules with delta table overrides, insert new derivations, rebuild delta, repeat until no new rows (max 100 iterations)
5. Extract results with optional constraint filtering

### `internal/datalog/compile.go` -- Table Override Support

Added `CompileOptions` struct with `TableOverrides map[string]string` field and `CompileWithOptions()` function. When a body atom's relation matches an override key, the compiler:
- Uses the override table name instead of `current_facts`/`facts`
- Skips the `relation = ?` WHERE clause (temp table is single-relation)
- Uses direct column references (`t0."col"`) instead of `json_extract()`
- Skips temporal scope filters (temp tables have no temporal columns)

`Compile()` remains backward-compatible, delegating to `CompileWithOptions` with empty options.

### `cmd/query.go` -- Wiring

Replaced in-memory evaluator fallback with `EvalRecursive`. Removed unused EDB construction (facts/catalog EDB only needed for in-memory eval).

### `internal/datalog/recursive_test.go` (~200 lines)

Five integration tests with real SQLite:
1. Transitive closure (ancestor from parent facts) -- verifies full 6-pair closure from 3-edge chain
2. Reachability (depends relation) -- same pattern, different domain
3. Constraint filtering on derived results
4. Fixpoint termination on small graph
5. Empty base case returns empty results

## Public API

```go
func EvalRecursive(db *database.CatalogDB, rules []Rule, relation string, constraints map[string]interface{}, scope TemporalScope) ([]Tuple, error)

type CompileOptions struct {
    TableOverrides map[string]string
}

func CompileWithOptions(rule Rule, scope TemporalScope, opts CompileOptions) (*CompiledQuery, error)
```
