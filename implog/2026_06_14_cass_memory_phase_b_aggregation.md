# cass-memory substrate — Phase B: Datalog aggregation

Date: 2026-06-14

## Summary

The one piece of genuine engine work in the cass-memory plan
(`docs/cass-memory-substrate-plan.md` §3.2): aggregate functions in Datalog rule
heads, compiled to SQL `GROUP BY`. This unblocks corroboration counts and the
Validator's evidence gate (e.g. "how many distinct sources called rule R harmful").

Non-recursive only — aggregating across a fixpoint needs stratification the
recursive evaluator does not implement, so recursive aggregation is rejected with a
clear error rather than silently mis-evaluated.

## Syntax

A head argument of the form `agg($Var)` becomes an aggregate term:

```
harmful_count: {
  head: {rel: "harmful_count", args: {target: "$T", n: "count($S)"}}
  body: [{rel: "feedback", args: {target: "$T", verdict: "harmful", source: "$S"}}]
}
```

Supported functions: `count`, `sum`, `min`, `max`. Non-aggregate head variables
become `GROUP BY` keys; aggregate terms reduce within each group. A head with only
aggregates and no group key reduces over the whole relation (single row).

## Changes

### `internal/datalog/types.go`
- `Term` gains an `Agg` field ("count"|"sum"|"min"|"max"), head-only.
- `Term.IsAggregate()` helper.
- `ParseTerm` recognizes `agg($Var)` via `aggTermPattern` regexp and produces
  `Term{Variable, Agg}`. Plain `$Var` and ground values unchanged.

### `internal/datalog/compile.go`
- Body atoms carrying an aggregate term are rejected ("aggregate not allowed in
  rule body").
- Head projection: aggregate terms emit `FN(expr) AS "key"`; non-aggregate head
  vars emit `expr AS "key"` and join the `GROUP BY` list. When any head term
  aggregates, the query uses `GROUP BY` (omitted if there are no group keys) and
  drops `DISTINCT`; otherwise the existing `SELECT DISTINCT` path is unchanged.

### `internal/datalog/query.go`
- `relationHasAggregateRule()` helper.
- Aggregation in a recursive rule → hard error (not supported).
- The empty-result recursive-retry safety net is skipped for aggregate relations:
  an empty aggregate (count over no matching facts) is a valid answer, not a miss,
  and the recursive evaluator cannot aggregate anyway.

## Public surface

Rule authors can now write `count($X)` / `sum($X)` / `min($X)` / `max($X)` in rule
heads. No API signature changes; `Evaluate`, `pudl query`, and `factstore.Query`
route aggregate rules through the existing non-recursive SQL path.

## Verification

- `internal/datalog/aggregate_test.go` (new): count-with-group-by, constraint
  filtering on an aggregate relation, pure aggregate (no group key), body-aggregate
  rejected, recursive-aggregate rejected. All pass.
- CLI e2e: feedback facts + an installed `harmful_count` rule →
  `pudl query harmful_count --json` returns `ruleX→2, ruleY→1` (helpful excluded).
- `CGO_ENABLED=0 go test ./...` full suite green.

## Notes / deferred

- `count($X)` compiles to `COUNT(expr)`, not `COUNT(DISTINCT expr)`. Content-
  addressed fact dedup means each (target, source) harmful feedback is one row, so
  counts are accurate in practice; a `count_distinct` variant can be added later if
  a use case needs it.
- Next: Phase C (decay view `current_facts_scored` / `fact_scored` EDB relation;
  `pow()` probe already green), then Phase D (FTS).
