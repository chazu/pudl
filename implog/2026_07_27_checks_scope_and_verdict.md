# Checks: evaluation coverage, scope, and verdict authority (D3)

**Date:** 2026-07-27
**Design:** `docs/design/2026-07-27-checks-scope-and-verdict.md`
**Closes:** D3 (all five points) from `docs/architecture-improvement-report.md`

## What was wrong

A converge run reporting `clean` had evaluated zero checks — the checks block
sat inside the `default:` observe-only branch — and even where checks did run,
a failing `fail`-severity check could not touch the persisted verdict. Checks
were also evaluated catalog-wide at wall-clock now while drift was computed over
a snapshot, and `--only` was invisible to them.

## What changed

**Checks run on every arm.** The block moved out of `default:` to after the mode
switch, so converge, differential drift, inventory drift, replay and populate
all evaluate them. Dry runs are exempt: `runChecks` borrows the run's catalog
handle, and that handle is lazy specifically so `--dry-run` never creates
`data/sqlite/catalog.db`. A check failure no longer displaces an earlier phase
error — `runErr` is first-error-wins.

**A failed `fail`-severity check demotes `clean` to `drifted`.** `runVerdict`
now wraps `phaseVerdict` and applies one rule to the *verdict*, not to a branch,
so the observe-only arm is covered as well as converge — D3 named only converge
because that is where it noticed the ordering, but `case r.Drift != nil` had the
identical hole. `drifted` rather than `failed`: the machinery worked, the
assertion about the resulting state did not. Only `clean` is demoted; `unknown`
(the run could not prove the state) and `""` (dry run / unverified replay) are
left alone. The demotion writes a note on the run row naming the checks
responsible and prints it live, so `drifted` is not mistaken for resource drift.

**Run scoping is opt-in by the rule author.** Constraints compile to
`WHERE "<key>" = ?` over the derived *head* columns, so passing `run_id` to a
relation whose head does not expose it is a SQL error, not a wider query. The
binding predicate is therefore `headExposesRunID`: bind only when some rule
producing the check's relation declares `run_id` as a variable head argument. No
shipped rule does, so this is a no-op until someone opts in. A `--from-catalog`
replay never binds it — a replay observes nothing, so binding its run ID would
make every `expect: empty` check pass trivially. `CheckResult.Scope` records
`"run"` or `"global"` so the difference is legible.

**`--only` partitions result tuples, fail-safe.** A row is advisory only when it
names at least one desired resource and *none* of the resources it names were
selected; an unresolvable row, or one naming both an included and an excluded
resource, gates. Partitioning applies to `expect: empty` checks only —
those count violations, so excusing out-of-scope ones is the point, whereas a
`nonempty` check counts evidence and dropping it could only manufacture a
failure nothing in scope can fix.

## Public API

Nothing exported changed outside `internal/`.

- `internal/acute` (new `scope.go`):
  - `type TupleScope`
  - `func NewTupleScope(original, effective *systemmodel.SystemModel) *TupleScope`
  - `func (*TupleScope) Restricted() bool`
  - `func (*TupleScope) Advisory(values []string) bool`
  - `func ArgValues(args map[string]interface{}) []string`
- `cmd` (unexported): `checkContext`, `headExposesRunID`, `partitionCheckTuples`,
  `phaseVerdict`, `failedFailSeverityNames`, `runFinishState.addNote`.
- `CheckResult` gained `AdvisoryCount` (`advisory_count`, omitempty) and `Scope`
  (`scope`). `Count` is now defined as *the number the verdict is about*;
  without `--only` that is unchanged from before.

## Tests

- `internal/acute/scope_test.go` — unrestricted runs classify nothing;
  out-of-scope rows are advisory; unresolvable and mixed rows gate; type-selector
  values gate whenever any resource of that type is in scope; short schema names
  resolve.
- `cmd/run_checks_scope_test.go` — partitioning per `expect`; `headExposesRunID`
  (including a ground head arg, which is not a binding point); verdict demotion
  across converge/drift/unknown/failed/replay/dry-run; advisory rendering does
  not gate; note accumulation.
- `cmd/run_checks_integration_test.go` — against a real catalog: the run-ID
  constraint scopes to this run, still sees this run's own rows (so scoping does
  not trivially pass), is never bound on a replay, and `--only` partitions real
  result tuples both ways.

The integration tests also settle the factual question D3's withdrawn draft got
wrong: a rule body binding `catalog_entry(run_id: $R, …)` with `$R` in the head
is constrainable to a run ID with no schema change at all.

## Not done here

End-to-end coverage that a *converge* run evaluates checks needs the mu Executor
seam extended to the populate/observe paths (Recommendation 1's remainder), so
the whole `RunE` can run without mu. Every piece is unit-covered; the wiring is
covered when that lands.

Binding a replay's `--catalog-scope` as a `collection_id`/`origin` constraint is
deferred: the replay scope is a union type classified per run, and no rule wants
it yet. The replay reports `scope: "global"` so the mismatch is visible.
