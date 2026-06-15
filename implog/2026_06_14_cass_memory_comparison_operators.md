# cass-memory substrate — Datalog comparison operators

Date: 2026-06-14

## Summary

Follow-up completing the Phase C recall story: numeric comparison constraints in
Datalog rule bodies, so a rule can express a recency-weighted recall gate like
`decayed_worth > 0.25` directly instead of thresholding in the consumer.

Supported operators: `>`, `<`, `>=`, `<=`, `!=`, each against a numeric literal.

## Syntax

A body-atom argument of the form `"OP number"` is a comparison constraint on that
column:

```
live: {
  head: {rel: "live", args: {id: "$I"}}
  body: [{rel: "fact_scored", args: {id: "$I", relation: "playbook", decayed_worth: ">0.25"}}]
}
```

## Changes

### `internal/datalog/types.go`
- `Term` gains a `Cmp` field (the operator) alongside the numeric bound in `Value`.
- `Term.IsComparison()` helper.
- `ParseTerm` recognizes `"OP number"` via `cmpTermPattern`
  (`^(>=|<=|!=|>|<)\s*(-?\d+(?:\.\d+)?)$`, two-char operators first) and produces
  `Term{Cmp, Value}`. `parseNumber` yields int64 when integral, else float64.

### `internal/datalog/compile.go`
- Body-atom processing: a comparison term emits `expr OP ?` (operator validated by
  the pattern, safe to inline) with the bound as a parameter. Variable and ground
  equality branches unchanged. Works for both json_extract (facts table) and
  native-column (EDB view, e.g. fact_scored) expressions.

Because the recursive evaluator also compiles rules via `CompileWithOptions`,
comparisons work in recursive rules too — no separate handling needed. `match.go`
(the top-level constraints filter) is unaffected.

## Verification

- `internal/datalog/comparison_test.go` (new): all five operators over an integer
  field (gt/gte/lt/lte/ne), plus a float-bound case.
- `internal/datalog/fact_scored_test.go`: `TestFactScoredThreshold` now uses
  `decayed_worth: ">0.25"` in the rule and asserts only the fresh fact survives the
  gate (stale ~0.125 filtered out).
- `CGO_ENABLED=0 go test ./...` full suite green.

## Notes

- Comparisons are against numeric literals only; variable-to-variable comparison
  (`$A > $B`) is not supported and is out of scope. Equality between variables
  remains expressed by sharing a variable name (the existing join mechanism).
