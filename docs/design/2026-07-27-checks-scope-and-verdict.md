# Design: model checks — evaluation coverage, scope, and verdict authority

**Date:** 2026-07-27
**Implements:** D3 (all five points) from `docs/architecture-improvement-report.md`
**Status:** stabilized after adversarial review (§7); ready to execute

## 1. The problem

D3 is the last live way `pudl run` can claim more verification than it
performed. Four distinct holes, one cluster:

1. **Converge runs evaluate no checks.** The checks block sits inside the
   `default:` observe-only branch (`cmd/run.go:256-266`), so a converge run
   reporting `clean` has evaluated zero checks.
2. **Checks cannot demote a verdict.** `runVerdict` derives status from
   `r.Converge` / `r.Drift` alone (`cmd/run.go:329-364`); a failing
   fail-severity check sets the exit code but never touches the persisted
   status. Hoisting checks without fixing this would let a converge run persist
   `clean` while a `fail` check failed.
3. **Checks are unscoped.** `runChecks` passes `nil` constraints
   (`cmd/run_checks.go:77`), so within one run the drift verdict is computed
   over `pr.SnapshotID` while check verdicts are computed over `current_facts`
   at wall-clock now — two different populations behind one report.
4. **`--only` is invisible to checks.** A check can fail on a resource this run
   deliberately excluded, and nothing in `CheckResult` records that.

## 2. Non-goals

- No `#SystemModel` schema change. Every mechanism below works off the rules
  already loadable and the tuples already returned.
- No new catalog columns. `catalog_entry_edb` already exposes `run_id`.
- No change to what `expect: empty|nonempty` means.

## 3. Decisions

### 3.1 Checks run on every non-dry-run arm

Move the checks block out of `default:` to after the mode switch. It then runs
for converge, differential drift, inventory drift, replay, and populate alike.

**Dry runs skip checks.** Not because checks mutate — they are read-only
Datalog — but because `runChecks` borrows `cat.required()`, and the run-owned
catalog handle is lazy precisely so that `--dry-run` never so much as creates
`data/sqlite/catalog.db` (see Recommendation 4's second slice). Evaluating
checks on a dry run would create the database file, converting a documented
"touches nothing" into "touches something small". A dry run also writes no
verdict, so the demotion in §3.2 would have nothing to demote.

**Checks still run when the phase already failed.** They are read-only and the
extra signal is free. `runErr` keeps the *first* error, so a check failure
cannot mask a converge failure.

### 3.2 A failed fail-severity check demotes `clean` to `drifted`

`runVerdict` gains the check results and applies one rule after computing the
phase verdict:

```
clean + any failed fail-severity check  ->  drifted
anything else                           ->  unchanged
```

Rationale for `drifted` over `failed`: the run's machinery did not fail — the
apply succeeded, the re-observation completed. What failed is an assertion
about the resulting state, which is exactly what `drifted` means in this
vocabulary (`unknown|drifted|converging|clean|failed`). Labelling it `failed`
would invite an operator to re-apply by hand, the same mistake D2 rejected for
lost receipts.

Only `clean` is demoted. `drifted` and `failed` are already at least as severe.
`unknown` is *not* demoted: it means "this run could not prove the state", and a
check evaluated over a catalog that may be missing the run's receipt cannot
upgrade that ignorance into knowledge. `""` (dry run, or an unverified
`--from-catalog` replay) stays `""` — Defect 3 established that a replay writes
no model status, and a check evaluated during a replay does not change the fact
that nothing was observed live.

**Scope note (found in review, not in D3's text):** D3 states the demotion for
converge only, because that is where it noticed `runVerdict` ordering. The same
hole exists on the drift arm: `case r.Drift != nil: if r.Drift.Clean { return
"clean" }` also ignores checks. Fixing only converge would leave the identical
defect one branch away. The rule above is applied to the verdict, not to a
branch, so both are covered.

### 3.3 Checks are scoped by an opt-in `run_id` constraint

`datalog.Evaluate`'s `constraints` map is applied as `WHERE "<key>" = ?` over
the *derived head columns* (`internal/datalog/sql_eval.go:43-59`). Passing a key
the head does not expose is therefore not "unscoped" — it is a SQL error. The
binding predicate must be checkable before the call:

> Bind `run_id` iff some loaded rule whose head relation is the check's `query`
> declares `run_id` as a **variable** head argument.

This makes scoping **opt-in by the rule author**. A rule that wants run scoping
writes `head: {rel: "...", args: {run_id: "$R", ...}}` with `catalog_entry(run_id:
$R, …)` in its body; a rule that wants catalog-wide evaluation simply does not
expose the argument, and behaves exactly as today. No existing rule exposes
`run_id`, so this change is a no-op until someone opts in — which is the correct
blast radius for a mechanism nobody has evidence for yet.

**`--from-catalog` never binds `run_id`.** A replay observes nothing, so no
catalog row carries this run's ID; binding it would make every `expect: empty`
check pass trivially — a false pass introduced by the fix meant to prevent false
passes. On a replay the constraint is skipped and the result is marked global.

**Deferred:** binding the replay's population (`--catalog-scope`) as a
`collection_id` / `origin` constraint. The replay scope is a union type
(snapshot ID *or* ingest origin, classified by `observeScopeFilter`), so the
binding would have to be classified per run, and no rule exists that wants it.
Deferred under the same "no evidence yet" rule applied to D4's relation. What is
*not* deferred is telling the operator: a replay's checks are reported with
`scope: "global"`, so the mismatch is visible rather than silent.

`CheckResult.Scope` records what actually happened: `"run"` when the constraint
was bound, `"global"` when it was not.

### 3.4 Under `--only`, result *tuples* are partitioned — fail-safe

D3 says to partition tuples rather than classify checks. The classifier:

> A tuple is **advisory** iff it resolves to at least one desired resource of the
> *original* model **and** none of the resources it resolves to are in the
> *effective* (selected) set. Every other tuple **gates**.

"Resolves to" reuses the existing selector namespace (`desiredSelectorValues` in
`internal/acute/plan.go`): a tuple resolves to a resource when any of the
tuple's argument values, stringified, appears in that resource's selector
values (identity keys, type keys, `metadata.name`, and the short name after
`#`).

Three properties this buys:

- **Fail-safe.** A tuple that resolves to nothing is of unknown scope and
  therefore gates. Only a tuple positively identified as belonging exclusively
  to excluded resources is downgraded. Ambiguity — a tuple resolving to both an
  included and an excluded resource — gates.
- **Zero behaviour change without `--only`.** Effective == Original, so every
  resolvable tuple resolves to a selected resource and nothing is advisory.
- **No ambiguity errors.** Unlike selector resolution (Defect 2), this asks a
  membership question, not an identity question, so a tuple matching several
  resources needs no disambiguation.

Note the classifier deliberately consults the *union* of identity and type
selector values. Using type values makes advisory-ness rarer (a tuple carrying
`kind: Deployment` resolves to every Deployment, so it gates whenever any
Deployment is in scope) — the fail-safe direction.

**Partitioning applies to `expect: empty` checks only.** See A9: an `empty`
check counts violations, so excusing out-of-scope ones is the whole point of
this rule; a `nonempty` check counts *evidence*, and removing evidence can only
manufacture a failure the operator cannot fix from within the scope. `nonempty`
checks therefore gate on the full result set, exactly as today, and report
`AdvisoryCount: 0`.

### 3.5 `CheckResult` carries the distinction

```go
type CheckResult struct {
    Name          string `json:"name"`
    Query         string `json:"query"`
    Severity      string `json:"severity"`
    Count         int    `json:"count"`                     // gating tuples
    AdvisoryCount int    `json:"advisory_count,omitempty"`  // out-of-scope tuples
    Scope         string `json:"scope"`                     // "run" | "global"
    Passed        bool   `json:"passed"`
    Message       string `json:"message,omitempty"`
}
```

`Count` keeps its meaning as *the number the verdict is about*. Without `--only`
that is unchanged from today; with `--only` the advisory remainder is reported
separately rather than folded in. `Passed` is computed from `Count` alone.

`printChecks` renders three states, so an operator never sees a `FAIL` whose
exit code was silently dropped:

```
  ✓ no-orphan-pods (warn)
  ✗ no-orphan-pods [fail] FAIL — 2 match(es): <message>
  ⚠ no-orphan-pods [fail] advisory — 3 match(es) outside --only scope: <message>
```

A check with both gating and advisory tuples renders the gating line and appends
`(+3 outside --only scope)`.

`anyFailSeverityFailed` reads `Passed`, which is already gating-only, so the
exit code follows the rendered verdict by construction.

## 4. Blast radius

| Surface | Change |
|---|---|
| `cmd/run_checks.go` | `CheckResult` fields, run-ID binding, tuple partitioning, rendering |
| `cmd/run.go` | checks hoisted out of `default:`; `runVerdict` takes checks |
| `internal/acute` | export a tuple-scope classifier built on `desiredSelectorValues` |
| `internal/datalog` | none |
| catalog schema | none |
| `#SystemModel` | none |

## 5. Tests

| Case | Assertion |
|---|---|
| Converge run with a passing check | check appears in the report; verdict `clean` |
| Converge run with a failing `fail` check | verdict `drifted`, exit non-zero, run row notes demotion |
| Converge run with a failing `warn` check | verdict `clean`, exit zero |
| Observe-only clean drift + failing `fail` check | verdict `drifted` (not `clean`) |
| `unknown` (needs-verification) + failing `fail` check | verdict stays `unknown` |
| Dry run with checks declared | no check evaluated, no catalog file created |
| Rule head exposes `run_id` | constraint bound, `Scope == "run"` |
| Rule head omits `run_id` | no constraint, `Scope == "global"`, same tuples as today |
| `--from-catalog` with a `run_id`-exposing rule | constraint *not* bound, `Scope == "global"` |
| `--only a` with tuples on `b` | those tuples advisory, `Passed` true, exit zero |
| `--only a` with tuples on `a` and `b` | `Count` 1, `AdvisoryCount` 1, `Passed` false |
| Tuple resolving to nothing | gates (fail-safe) |
| Tuple resolving to both in and out of scope | gates |

## 6. Sequencing

Self-contained. Depends on nothing outstanding; blocks nothing. The run record
(Defect 1) it writes its demotion note to already exists.

## 7. Adversarial review

Attacks run against §3 before implementation. Three landed and changed the
design; the rest are recorded so the reasoning is not re-litigated.

**A1 — "Binding `run_id` unconditionally is the obvious reading of D3, and it is
a false-pass generator."** *Landed.* An `expect: empty` check constrained to a
run ID that nothing in the catalog carries passes trivially. This is fatal on
the replay path (nothing ever carries the replay's run ID) and merely surprising
on live paths. Resolution: the binding predicate is the rule head exposing
`run_id` as a variable (§3.3), which makes it opt-in and makes a trivially-empty
result the rule author's explicit choice; and the replay path never binds.

**A2 — "D3 only asks to demote the converge verdict; demoting the drift verdict
is scope creep."** *Landed, against D3's letter.* The drift arm has the
identical defect. Shipping the narrow fix would leave `pudl run m` (no
`--converge`) able to persist `clean` with a failed `fail` check — the same
unearned `clean` this cluster exists to close, one branch away. Recorded in
§3.2 as an explicit extension rather than done quietly.

**A3 — "Partitioning by 'tuple arg value appears in the resource's selector
namespace' will misclassify."** *Partially landed.* It will: a tuple arg that
coincidentally equals a resource name resolves to that resource. But every
misclassification is in the gating direction, because advisory requires *all*
resolutions to be out of scope and an unresolvable tuple gates. The failure mode
is "an out-of-scope tuple gates anyway", which produces a spurious failure the
operator can see and explain — not a silently dropped one. Documented as the
deliberate bias in §3.4 rather than engineered away.

**A4 — "`Count` changing meaning under `--only` breaks JSON consumers."**
*Rejected.* It changes meaning only when `--only` is passed, which today
produces no check evaluation at all on the converge arm and an unpartitioned
count on the observe arm. There is no consumer of a partitioned count because
partitioning does not exist yet. The alternative — `count` as the total plus a
separate gating field — makes the *verdict* field the non-obvious one, which is
worse for the reader who checks `passed` and `count` together.

**A5 — "Skipping checks on a dry run means `--dry-run` under-reports."**
*Rejected, with a caveat recorded.* A dry run reports `Mode: dry-run` and writes
no verdict, so no check result would be load-bearing. Against that: evaluating
them would create the catalog file on a path documented to create nothing. The
caveat is real — an operator cannot preview check results — and the answer is
that a plain `pudl run <model>` (observe-only, no `--converge`) already
evaluates checks without applying anything, so the capability exists on a path
that is honest about touching the catalog.

**A6 — "Running checks after a failed converge produces noise that buries the
real error."** *Rejected.* `runErr` is first-error-wins, so the converge error
is what surfaces and what the run row records. The check lines are additional
report content, not a replacement.

**A7 — "Demoting to `drifted` collides with `modelRowVerdict`'s `--only`
handling."** *Checked, no collision.* `runVerdict` produces `drifted`;
`modelRowVerdict` degrades only `clean`, so `drifted` passes through and
generalizes onto the model row — which is the documented rule ("a defect in a
subset is a defect in the model"). Under `--only` an advisory tuple never
produces that `drifted` in the first place, so the generalization is only ever
made off an in-scope failure.

**A9 — "Partitioning a `nonempty` check inverts the fail-safe argument."**
*Landed, found while mapping §3.4 onto the code.* For `expect: empty`, moving a
tuple to advisory makes the check more likely to pass — which is the intent, and
the fail-safe bias in §3.4 keeps unclassifiable tuples gating. For `expect:
nonempty`, moving a tuple to advisory makes the check more likely to **fail**:
`--only db` against a check asserting "at least one app replica runs" would drop
the satisfying tuple and manufacture a failure that is not about `db` and cannot
be fixed by anything in scope. Passing on out-of-scope evidence is not an
unearned claim either — the check asserts that something exists, and it does.
Resolution: partition `empty` checks only; `nonempty` gates on the full set.
This also removes a genuinely confusing field interaction, where
`AdvisoryCount` would have been *excluded* from `Count` under one `expect` and
*included* under the other.

**A8 — "The demotion is invisible: an operator sees `drifted` and hunts for
resource drift that isn't there."** *Landed.* The demotion writes a note on the
run row naming the checks responsible, and prints it live, in the same shape as
the existing `--only` scope note.
