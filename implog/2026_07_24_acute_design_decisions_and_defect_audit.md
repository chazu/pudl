# ACUTE design decisions settled + defect audit

Closed the six open design questions in `docs/architecture-improvement-report.md`
(Recommendation 1) and recorded the defects found while settling them. No code
changes — this is a design-decision and audit pass.

## What changed

`docs/architecture-improvement-report.md`:

- Replaced "Design questions for discussion" and "Recommended defaults for the
  first slice" with **D1-D6 as settled decisions**, each carrying its reasoning.
- Added a **15-entry defect register**, severity-ranked, every entry grounded in
  a file:line citation. Entries marked *(reproduced)* were demonstrated with
  temporary probe tests that were deleted afterwards.
- Re-ordered "Suggested sequence" by blast radius: wrong-target mutation first,
  then the false-`clean` cluster.
- Fixed a contradiction in "Implemented first slice", which still promised
  resumable recovery that D1 rejects.

## The decisions

| | Decision |
|---|---|
| D1 | No general resume; build a durable run record with a terminal marker. Carries two obligations: the iteration cap must become durable, and re-running must stop overwriting provenance. |
| D2 | Post-apply persistence failure is `unknown` at the resource level, reason carried on the run record. Blocked on D1. |
| D3 | Converge runs must evaluate checks; a fail-severity check demotes the converge verdict; checks are scoped by Datalog *constraint*, and `--only` partitions result *tuples* rather than classifying checks. |
| D4 | Ship the typed `depends_on` field for correctness; defer the derived relation and justify it separately. |
| D5 | Rollback out of scope for V1, and never as inverse-action synthesis. |
| D6 | Session pre-allocates both run and snapshot IDs; rows commit on completion. |

## Adversarial review

Three independent reviewers attacked the first draft along separate axes —
factual claims, reasoning, completeness. The review changed the substance rather
than polishing it, and the corrections are recorded in the document rather than
quietly applied:

- **D3's original framing was falsified.** It claimed checks could not be scoped
  because there was "nothing scope-shaped to pass". `datalog.Evaluate` takes a
  constraints map and the run path already uses it to scope a catalog-wide
  relation to the current model (`cmd/run_depends.go:161-162`), and
  `catalog_entry_edb` exposes `run_id`. D3 was rewritten around scoping by
  constraint.
- **Defect 1's original claim was refuted.** "A crashed run records nothing at
  all" is false — `recordModelInstance` (`cmd/run.go:89`) writes a run-ID-stamped
  snapshot and model-instance row before any phase. The real defect is the
  absence of a *terminal* marker plus status preservation in `UpsertEntry`
  (`internal/database/catalog.go:693-700`), which lets a stale `clean` survive an
  interrupted converge. Sharper than the withdrawn claim.
- **D1 and D2 were arguing opposite sides of idempotence** and are now
  reconciled: D1's "re-running is safe" concerns the loop, which re-observes
  before it applies; D2's "retry is unsafe" concerns a bare re-apply that skips
  that observation.
- **D4, D5 and D6 kept their conclusions but lost their justifications.** D4's
  machinery reuse was false economy (compound identity, cross-model name
  collisions, differing cardinality regime); D5's "permanently" overreached its
  premise; D6's stated guarantee ("no snapshot row for an incomplete
  observation") is a property the code does not have.

The audit also grew the register from 3 defects to 15, including four the first
pass missed entirely: unscoped `--from-catalog`, replay-driven `converging ->
clean` promotion, dry-run impurity, and lost-receipt masking on three of four
convergence exit routes.

## Public API

None. Documentation only.
