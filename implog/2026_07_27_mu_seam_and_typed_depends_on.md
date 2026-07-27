# The mu subprocess seam, and a typed resource `depends_on` (Rec 1 remainder, D4)

**Date:** 2026-07-27
**Closes:** Recommendation 1's outstanding adapter coverage, and D4, from
`docs/architecture-improvement-report.md`

## What was wrong

`acute.Executor` abstracted the converge loop's observe/plan/apply, but its
production implementation — and every other phase — reached
`exec.Command("mu", …)` directly. So the acceptance matrix the report specifies
("observe-only differential run", "converge to clean", "apply failure",
"manifest persistence failure", …) could not run end to end without a real mu, a
cluster and a network, and none of those rows had a test.

Separately, resource-level `depends_on` was untyped: three accepted spellings
(`depends_on`/`dependsOn`, bare string or list), no declaration in
`#SystemModel`, and a resolution rule that lived only in the reader.

## What changed

**A `muRunner` seam.** One interface with `Observe(config, target)` and
`Build(config, target, flags...)`, covering all five direct invocations — the
plugin-observe populate, the ewe populate build, the differential drift observe,
and the converge loop's plan and apply. `execMu` is the production runner,
preserving the argument order and error wrapping exactly.

It is passed explicitly rather than kept in a package variable. A swappable
global would be a shared mutable seam that parallel tests race on; the cost of a
parameter is one word at four call sites. `reconcileWorkspace` carries it as a
field, which is where the converge path already keeps its other run-scoped state.

**The acceptance matrix now runs.** `cmd/run_acceptance_test.go` drives the real
`runDrift` and `runConvergeLoop` against a scripted runner: observe-only clean
and drifted; converge to clean (observe → apply → re-observe); apply failure
(`failed`, zero iterations); manifest persistence failure (needs-verification →
`unknown`, not `clean` and not `failed`); dry run (plans, applies nothing, does
not open the catalog); an exhausted apply budget (observes, applies nothing);
and an observe failure *after* an apply (needs-verification).

**`depends_on` is typed.** `#DesiredResource` declares `depends_on?: [...string]`
with the resolution rule stated in the schema itself. The rule:

- Each entry is a selector resolved against this model's desired list, and must
  resolve to **exactly one resource by an identity key** (`name`, `id`, `path`,
  `target`, `metadata.name`, or a short name after `#`).
- A **type key** (`_schema`, `schema`, `definition`, `kind`) is an error *as a
  class*, not on cardinality. A type key matching one resource today would
  silently become two edges when a second resource of that type is added — the
  Defect 2 failure mode one level down. `--only` still accepts type keys; a
  dependency does not.
- A string that names one resource by identity *and* others by type stays an
  error. Identity would win, but silently preferring it is what Defect 2
  rejected: the two readings select different sets and the author cannot tell
  which they got.

D4's note about the compound identity is answered by saying what a dependency is
*not*: `recordIdentity`'s `<schema>|<field>/<field>` is schema-relative and only
exists once a record has been observed, whereas a dependency is declared before
anything is observed. It is stated in the model's own terms instead.

Resource dependencies remain deliberately unemitted as facts, per D4: `--only`
resolves them at plan time from the loaded model, and deriving converge scope
from catalog facts would make it depend on mutable state.

## Public API

- `cmd` (unexported): `muRunner`, `execMu`; `runPopulate`, `runEwePopulate`,
  `runDrift`, `runConvergeLoop` and `setupReconcileWorkspace` take a runner;
  `reconcileWorkspace.Mu`.
- `internal/systemmodel/schema.cue`: `#DesiredResource`, with `depends_on`.
- `internal/acute`: `resolveDependency` resolves by identity key only.

## Breaking change

`depends_on: "x"` (bare string) and `dependsOn:` are no longer read, and CUE now
rejects the first. A model using either fails at load rather than silently having
its dependency dropped at plan time, which is the point of typing the field.

## Not done here

D4's optional half — emitting a `resource_depends_on` relation — stays deferred
on D4's own reasoning: resource names are not unique across models, the
cardinality regime differs by orders of magnitude from the model-level relation,
and feeding the existing `impacted_by` rules would mean either corrupting their
arg-key contract or duplicating all three recursive rules. It is justified on
query value if it is ever revived, not on economy.
