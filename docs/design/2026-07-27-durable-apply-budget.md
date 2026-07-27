# Design: a durable apply budget — making the halting guarantee survive re-running

**Date:** 2026-07-27
**Implements:** D1's first obligation from `docs/architecture-improvement-report.md`
**Status:** stabilized after adversarial review (§7); ready to execute

## 1. The problem

D1 decided against a general resume/checkpoint state machine, on the grounds
that the ACUTE cycle begins with an observe and therefore re-derives what a
checkpoint would have told it. That decision came with an obligation:

> **The iteration cap must become durable.** `MaxIterations` is per-process
> (`internal/acute/coordinator.go:103`), while `#SystemModel` carries
> `freshness.every` — runs are designed to be scheduled. A resource that
> oscillates (apply succeeds, drifts back) gets a cap of N *per run* and an
> unbounded global apply rate under any scheduler or crash-loop supervisor.
> Re-running is only safe if the halting guarantee survives it, so the cap must
> consult durable attempt history per model.

Today `MaxIterations` bounds applies within one process and nothing bounds them
across processes. A model that cannot converge, scheduled every five minutes,
applies 5 times every 5 minutes forever. A crash-loop supervisor restarting a
run that dies mid-converge is worse: each restart gets a fresh cap.

## 2. The shape of the fix

The durable cap is **the same policy one level up**. The per-run cap says "stop
after N applies that did not reach clean". The durable cap says the same thing
over the model's history:

> **Bound the applies a model may perform since it was last verified clean.**

This is the right quantity for three reasons:

- It is what D1 names as unbounded: the *apply rate*.
- It discriminates correctly between a healthy remediation loop and an
  oscillation. A model that drifts and is fixed every run **ends each run clean**,
  so its count resets to zero every run and it can run forever. A model that
  applies and still is not clean accumulates.
- It resets on evidence, not on a timer, so no window parameter has to be
  invented or tuned.

### 2.1 What counts, what resets

- **Counted:** every successful `mu` apply, recorded durably *at the moment it
  succeeds* — not at the end of the run. Crash-loop supervision is explicitly in
  the threat model, so a count that only lands in a terminal `FinishRun` would
  be zero exactly in the case the budget exists to stop.
- **Resets:** the first run, walking back from newest, whose verdict is `clean`
  **and which was not scoped by `--only`**.

The scope condition matters. A scoped `clean` already does not generalize onto
the model row (`modelRowVerdict` degrades it to `unknown`, because "a scoped ∅
does not prove the whole model clean"). Letting it reset the budget would open
exactly the hole that reasoning closes: a scheduler alternating `--only app`
(converges clean) with `--only db` (oscillates) would reset the budget every
other run and sustain the unbounded apply rate on `db`. The same rule, applied
to the same question, gives the same answer.

### 2.2 What a budget-exhausted run does

It **still observes**. Observation is how the budget resets — if the model has
since become clean, the run ends `clean`, the budget resets, and nothing was
refused. Only the apply is withheld.

Outcome `failed (apply_budget_exhausted)`, verdict `failed`, and a run-row note
naming the count and the escape hatch.

### 2.3 The escape hatch

`--max-applies 0` means unlimited. There is deliberately no separate "reset"
command and no stored breaker state to clear: the budget is derived from run
history on every run, so the only ways to change it are to converge the model
(which is the point) or to say `--max-applies 0` (which is one flag, auditable
in the run's own record).

Default: **20**, four runs at the default `--max-iters 5`.

## 3. Where the policy lives

The coordinator, so it is testable with fake executors and so the decision to
withhold an apply is visible in the same place as the decision to make one.

```go
type ConvergeRequest struct {
    // ...
    // ApplyBudget caps applies across the model's history since it was last
    // verified clean. nil means no durable budget is known (no catalog), which
    // preserves the previous behaviour exactly.
    ApplyBudget *int
    // OnApplied fires immediately after each successful apply, before the
    // manifest is recorded, so the durable count cannot be lost to a later
    // failure in the same iteration.
    OnApplied ApplyHook
}
```

`ApplyBudget` is a pointer rather than a sentinel because "unknown" and "zero"
are genuinely different states and the second is reachable: a budget of exactly
zero must observe-then-refuse, while an unknown budget must not constrain
anything.

`MaxIterations >= 1` stays as it is. The two caps are independent and the loop
stops at whichever binds first.

## 4. Schema

Two columns on `runs`, both idempotent additions in the established style:

| Column | Meaning |
|---|---|
| `applies INTEGER NOT NULL DEFAULT 0` | successful applies this run performed |
| `scoped INTEGER NOT NULL DEFAULT 0` | the run was restricted by `--only` |

New catalog operations:

```go
func (c *CatalogDB) RecordApply(runID string) error            // applies = applies + 1
func (c *CatalogDB) AppliesSinceLastClean(model string) (int, error)
```

`AppliesSinceLastClean` walks the model's runs newest-first, summing `applies`,
and stops at the first unscoped `clean` verdict. Unfinished rows count: a run
that died after applying twice contributes two, which is the entire reason the
count is incremented per apply.

## 5. Tests

| Case | Assertion |
|---|---|
| No history | full budget; behaviour identical to today |
| Nil budget (no catalog) | no constraint; previous behaviour preserved |
| Budget > applies needed | converges normally |
| Budget exactly exhausted mid-loop | applies stop at the budget, not at `--max-iters` |
| Budget 0, model dirty | observes, applies nothing, outcome `apply_budget_exhausted` |
| Budget 0, model clean | observes, ends `clean` — the budget never bites |
| `RecordApply` per iteration | count lands before the manifest is recorded |
| Apply then crash (no `FinishRun`) | the applies are still counted |
| History with an unscoped clean | sums only runs after it |
| History with a *scoped* clean | does **not** reset; sums through it |
| `--max-applies 0` | unlimited, recorded on the run row |

## 6. Sequencing

Self-contained. Uses the run record from Defect 1, which exists.

## 7. Adversarial review

**A1 — "The budget will stop a legitimate remediation loop."** *Rejected, and it
is the design's central discrimination.* A remediation loop ends each run
`clean` (the converge loop's final re-observation is what produces that
verdict), so its count resets every run. The only model that accumulates is one
that applied and *still* was not clean — which is the definition of the case D1
wants halted. Recorded because it is the first objection anyone will raise.

**A2 — "Counting at `FinishRun` is simpler than a write per apply."** *Rejected.*
D1 names crash-loop supervisors specifically. A run killed mid-converge never
reaches `FinishRun`, so a terminal-only count is exactly zero in the scenario the
budget exists for, and the supervisor's next restart gets a full budget. The
write is one small `UPDATE` per apply, against a loop that shells out to mu for
minutes; the cost is not measurable.

**A3 — "A scoped clean not resetting is over-strict; operators will be confused
when `--only app` converges and the budget stays put."** *Landed as a
documentation obligation, not a design change.* The alternating-scope hole is
real (§2.1) and the strict rule is the same one `modelRowVerdict` already
applies. The confusion is answered by saying so: the exhaustion message names
the count and states that an unscoped clean run resets it.

**A4 — "`ApplyBudget *int` is un-Go-ish; use -1 for unlimited."** *Rejected.*
Zero is a reachable, meaningful budget with different behaviour from unlimited
(observe-then-refuse versus do not constrain). A sentinel makes the two look
like neighbours on a number line when they are different modes, and it makes
`ApplyBudget: 0` — the zero value of the struct field — silently mean "refuse
every apply" for any caller that forgets to set it. The pointer makes the
default state "unknown", which is the safe one.

**A5 — "The budget is computed once at run start, so a long converge can exceed
it."** *Checked, cannot happen.* The budget is passed to the coordinator as a
count of applies *this run may still make*, and the coordinator decrements
against its own `Iterations`. Concurrent runs of the same model could each be
granted the same budget, but concurrent converges of one model are already
unsound for reasons that have nothing to do with this (two `mu build`s against
one target), and the durable count still records both, so the *next* run sees
the true total.

**A6 — "Verdict `failed` for an exhausted budget contradicts D2, which rejected
`failed` for a state the operator might re-apply by hand."** *Rejected on the
distinction D2 itself draws.* D2's objection was to labelling a *successful*
apply as `failed`, which invites a manual re-apply of something already applied.
Here nothing was applied and the model is genuinely not converging; `failed` is
accurate, and the cap-exhausted route already uses it. The existing
`NeedsVerification` rule still dominates: an exhausted budget on a run that also
lost a receipt yields `unknown`, because `runVerdict` checks needs-verification
first.

**A7 — "Walking the whole run history on every run is a scan."** *Accepted as
negligible, with a bound.* The walk stops at the first unscoped clean, which for
a healthy model is the previous run. The pathological case — a model that has
never been clean — walks its whole history, which is bounded by how many times
someone has run it, and the `idx_runs_model` index already orders by
`(model, started_at)`. If it ever matters, the query takes a LIMIT.
