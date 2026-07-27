# A durable apply budget: making the halting guarantee survive re-running (D1)

**Date:** 2026-07-27
**Design:** `docs/design/2026-07-27-durable-apply-budget.md`
**Closes:** D1's first obligation from `docs/architecture-improvement-report.md`

## What was wrong

`MaxIterations` bounded applies within one process and nothing bounded them
across processes. `#SystemModel` carries `freshness.every`, so runs are designed
to be scheduled: a model that cannot converge applied 5 times every 5 minutes,
forever. A crash-loop supervisor was worse — each restart got a fresh cap, and
the applies its predecessors had already made were recorded nowhere.

D1 rejected a general resume/checkpoint state machine on the grounds that the
ACUTE cycle begins with an observe and re-derives what a checkpoint would say.
That is only safe if the halting guarantee survives re-running, which is what
this adds.

## What changed

**The durable cap is the same policy one level up.** The per-run cap stops after
N applies that did not reach clean; the durable cap bounds the applies a model
may make *since it was last verified clean*. That quantity discriminates
correctly: a model that drifts and is fixed every run ends each run `clean`, so
it resets every run and can run forever; only a model that applied and still is
not clean accumulates.

**Counted per apply, not at the end.** `RecordApply` increments the run row the
moment an apply succeeds, before its manifest is recorded. Writing the count at
`FinishRun` would leave it at zero for a run killed mid-converge — exactly the
case the budget exists to bound.

**A scoped clean does not reset it.** A `--only` clean does not generalize onto
the model (`modelRowVerdict` already degrades it to `unknown` for this reason),
and treating it as a reset would let a scheduler alternating a converging scope
with an oscillating one refill the budget every other run.

**Exhaustion withholds the apply, not the observation.** Observing is how the
budget resets, so a budget of zero still observes; if the model has since become
clean the run ends `clean` and nothing was refused. Otherwise the outcome is
`failed (apply_budget_exhausted)`, which `runVerdict` maps to `failed` — and
`NeedsVerification` still dominates it, so a run that also lost a receipt is
`unknown`.

**The catalog failing open, not closed.** A budget that cannot be read yields
`nil` — no constraint — because silently refusing to apply on an unreadable
catalog is a worse failure than the one being prevented.

## Public API

- `internal/database`:
  - `RunRecord` gained `Applies int` and `Scoped bool`.
  - `FinishRun(runID string, conclusion RunConclusion) error` — signature
    changed; the new `RunConclusion` struct carries verdict, outcome,
    needs-verification, note and scope. The apply count is deliberately not in
    it.
  - `func (c *CatalogDB) RecordApply(runID string) error`
  - `func (c *CatalogDB) AppliesSinceLastClean(model string) (int, error)`
  - `runs` gained `applies` and `scoped` columns, added idempotently so an
    existing database migrates on open.
- `internal/acute`:
  - `OutcomeBudgetExhausted Outcome = "failed (apply_budget_exhausted)"`
  - `ConvergeRequest.ApplyBudget *int` — nil means unknown/unconstrained; zero
    means observe-then-refuse. A pointer rather than a sentinel because the
    struct's zero value must not mean "refuse every apply".
  - `ConvergeRequest.OnApplied ApplyHook` — fires after each successful apply,
    before the manifest is recorded.
- `cmd`: `--max-applies` (default 20, `0` disables, requires `--converge`);
  unexported `resolveApplyBudget`, `recordDurableApply`;
  `runConvergeLoop` takes the budget; `runFinishState.scoped`.

## Tests

- `internal/acute/apply_budget_test.go` — nil budget preserves prior behaviour;
  the budget binds before the run cap and vice versa; a zero budget observes and
  refuses; a zero budget on a clean model ends clean; `OnApplied` fires per
  successful apply and before the manifest, and not at all for a failed one; a
  lost receipt still dominates an exhausted budget.
- `internal/database/catalog_apply_budget_test.go` — no history; sum until the
  last unscoped clean; a scoped clean does not stop the walk; unfinished runs are
  counted (the crash case); other models ignored; `RecordApply` on an unknown
  run errors; the column migration runs on an existing table.
- `cmd/run_apply_budget_test.go` — observe-only, dry-run and `--max-applies 0`
  return no budget *and do not open the catalog*; a fresh model gets the full
  budget; spent applies subtract; exhaustion floors at zero; a clean run refills;
  an unreadable catalog does not refuse to apply; flag validation.

## Not done here

Nothing in D1 remains: the terminal run marker landed with Defect 1 and
provenance with Defect 7. The report's "no general resume" position is unchanged
and unaffected.
