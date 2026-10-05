# Datalog: strict rules, head semantics, goal-directed evaluation

**Date:** 2026-10-05
**Branch:** `improve/datalog`

Three changes to `internal/datalog`, each fixing behaviour reproduced against
the shipped binary.

## E — Malformed rules are errors, not silence

Before: `ParseRules` did `continue // skip non-rule fields` on any extraction
error. A rule with a typo (e.g. a body atom missing `rel`) vanished, and every
query depending on it returned "No results." — a model check reads that as a
pass.

Now:

- A top-level struct with `head` or `body` is a rule; other fields are ignored.
- A rule that cannot be read, or fails range restriction (head variable not
  bound in the body), comparison-in-head, or aggregate-in-body, is kept in the
  rule set with `Rule.LoadErr` and `Rule.Source` (`file:line:col`).
- `Evaluate` fails a query whose dependency closure contains an invalid rule.
  An invalid rule with an unreadable head blocks every query.
- `pudl rule add` rejects files with invalid rules; `pudl rule new` warns about
  invalid neighbours on stderr; `pudl doctor` has a **Datalog Rules** check
  (`internal/doctor/rules_check.go`).

## F — Head semantics

- Constant head arguments (`severity: "high"`) were skipped: missing from
  results, and filtering on them failed with `no such column`. They are now
  projected as bound parameters in the SQL compiler and fixpoint temp tables.
- Rules for one relation were joined with `UNION ALL` and aligned by the first
  rule's keys, so a tuple derived twice was returned twice. They are now
  joined with `UNION`; all rules of a relation must share one head key set.
- `quoteIdent` quotes every identifier taken from rule files (head keys, temp
  table names, constraint keys).
- Argument keys came from `Selector().String()`, which keeps CUE quoting: a
  label written `"app-name"` became a key with literal quote characters. Keys
  are now unquoted; JSON paths quote the label (`$."a.b"`) so a dotted key is
  one key. Keys containing `"` are rejected.

## G — Stratified, goal-directed evaluation

Before: any rule reading a derived relation counted as "recursive". Querying a
recursive relation ran a fixpoint over **every** derived relation, and a
zero-row SQL answer re-ran that whole fixpoint (a "safety net" that could never
find anything, because SQL-routed relations only read stored facts). A
`count` over any derived relation was rejected as recursive aggregation.

Now (`strata.go`, `eval_acyclic.go`, `eval_stratified.go`, `edb.go`):

1. Plan only the queried relation's dependency closure.
2. Group it into strongly connected components (Tarjan) in dependency order.
3. No cycles: one SQL statement, each derived relation a CTE (via the existing
   `TableOverrides` mechanism). Cycles: materialize components into temp
   tables in order, iterating only cyclic components semi-naively.
4. Aggregates are allowed in any acyclic component.
5. The iteration cap is `DefaultMaxIterations` (100), overridable with
   `EvalOptions.MaxIterations` / `pudl query --max-iterations`.

`recursive.go`, `sql_eval.go`, `partition.go` (`EvalRecursive`,
`SQLEvaluator`, `PartitionRules`) are removed; they were internal-only.

### Verification

- Differential test (`differential_test.go`): 300 random programs (joins,
  constants, multi-rule relations, recursion, mutual recursion), 2400
  comparisons (57% non-empty, 88% of programs recursive) against the frozen
  pre-G evaluator in `oracle_test.go`. All agree.
- Benchmarks (`bench_test.go`, 10x, 3 runs; machine shared with other jobs):

| Benchmark | Before | After |
|---|---|---|
| LayeredRules (3 layers over 300 edges) | 209–615 ms/op | 198–204 ms/op |
| TransitiveClosureFiltered (+ unrelated recursive relation) | 122–133 ms/op | 4.0–4.2 ms/op |
| CheckZeroRows (check matching nothing, recursive rules present) | 57–132 ms/op | 0.09–0.10 ms/op |

  LayeredRules is dominated by unindexed JSON-argument joins over base facts,
  which routing does not change.
- Regression: with a 120-step unrelated chain, a zero-row check previously
  **failed** ("fixpoint not reached after 100 iterations"); it now succeeds
  (`TestUnrelatedDeepRecursionDoesNotAffectQuery`).

## Public API

- `datalog.Rule` gains `Source string`, `LoadErr error`, `Valid()`, `Describe()`.
- `datalog.InvalidRules`, `datalog.RuleSetProblems`.
- `datalog.EvaluateWithOptions`, `datalog.EvalOptions{MaxIterations}`,
  `datalog.DefaultMaxIterations`. `Evaluate` keeps its signature.
- `pkg/eval`: `InvalidRules`, `RuleSetProblems`; `LoadRulesFromPaths` /
  `ParseRulesFromSource` now return invalid rules with `LoadErr` set.
- `doctor.CheckRulesAt(paths ...string)`.
- `pudl query --max-iterations`.

## Behaviour changes

- Workspaces with latent broken rules now see errors from queries that depend
  on them, and from `pudl doctor`.
- Relations whose rules disagree on head keys now fail instead of returning
  misaligned unions.
- Derived tuples include constant head keys.
- Arg keys written as quoted CUE labels lose their literal quotes in results.

## Deferred

- Expression indexes for hot JSON argument keys (the remaining cost in
  LayeredRules).
- `QueryCurrentFactsFiltered` (database package) still builds `$.key` paths
  for EDB constraints, so a dotted key in a constraint on a plain fact
  relation is read as a nested path.
- `factstore.Store.Query` does not expose `EvalOptions`.
- `pudl query --list` does not yet list invalid rules (doctor does).
