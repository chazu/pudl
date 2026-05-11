# SQL Query Compiler Implementation

**Date:** 2026-05-10

## Summary

Implemented a SQL compilation backend for pudl's datalog evaluator. Non-recursive datalog rules are now compiled to parameterized SQL queries and executed directly against SQLite, bypassing the in-memory semi-naive fixed-point evaluator for those rules.

## Files Created

- `internal/datalog/compile.go` — Core compiler: `Compile(rule Rule, scope TemporalScope) (*CompiledQuery, error)`. Each body atom becomes a self-join on `current_facts` (or `facts` for temporal queries) with aliases `t0`, `t1`, etc. Shared variables across atoms become equi-joins via `json_extract`. Ground terms become parameterized WHERE clauses.

- `internal/datalog/compile_test.go` — Tests for single atom, two-atom join, ground terms, temporal scope (valid-only and bitemporal), head subset projection, three-way join, and error cases.

- `internal/datalog/sql_eval.go` — `SQLEvaluator` that finds rules matching a queried relation, compiles them, executes SQL, decodes results into `[]Tuple`. Multiple rules for same head use UNION ALL. Falls back to direct `QueryCurrentFacts`/`QueryCurrentFactsFiltered` for base relations with no matching rules.

- `internal/datalog/sql_eval_test.go` — Integration tests with real SQLite: simple fact lookup, single-rule derivation, multi-join, constraint filtering, EDB fallback, and temporal mode.

- `internal/datalog/partition.go` — `PartitionRules(rules []Rule) (recursive, nonRecursive []Rule)`. A rule is recursive if any body atom's relation appears as any rule's head relation.

## Files Modified

- `internal/database/catalog.go` — Added `DB() *sql.DB` accessor for direct query execution.

- `cmd/query.go` — Wired `SQLEvaluator` for non-recursive rules. Partitions rules, uses SQL eval for non-recursive, falls back to in-memory `Evaluator` for recursive rules.

## Public API

```go
// Compiler
func Compile(rule Rule, scope TemporalScope) (*CompiledQuery, error)

// SQL Evaluator
func NewSQLEvaluator(db *database.CatalogDB, rules []Rule, scope TemporalScope) *SQLEvaluator
func (e *SQLEvaluator) Query(relation string, constraints map[string]interface{}) ([]Tuple, error)

// Rule partitioning
func PartitionRules(rules []Rule) (recursive, nonRecursive []Rule)

// Types
type TemporalScope struct { ValidAt, TxAt *int64 }
type CompiledQuery struct { SQL string; Params []interface{}; Head Atom; Vars map[string]string }

// Database accessor
func (c *CatalogDB) DB() *sql.DB
```

## Deferred

- Multi-source joins (catalog_entries + facts)
- Negation (NOT EXISTS)
- Aggregates
- Prepared statement caching
- SQL temp tables for recursive rules
