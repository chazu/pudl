# Wiring documentation and init inventory

**Date:** 2026-08-05

> **Resolved later that day:** mu plan schema v2 now projects strict sealed
> action claims, and PUDL's real producer/consumer approval-resume smoke passes.
> The v0.3.3/`pudl-olm` language below records the earlier discovery point.

## Outcome

PUDL and mu's living documentation and embedded usage text were updated for the
`#SystemModel`, exact `run-set`, plain-binding, and accepted strict
sealed-routing contracts. Removed `pudl drift` / `pudl export-actions` /
`pudl process` usage was replaced with the current commands; historical records
remain snapshots. A later live CLI sweep found that the strict sealed run-set
contract is not executable against mu v0.3.3 because its JSON plan projection
omits action-level sealed claims; PUDL fails that path closed as `pudl-olm`.

PUDL initialization no longer maintains a handwritten subset of built-in files.
Fresh installation and existing-workspace repair share an inventory derived from
the complete embedded schema tree plus the programmatic
`pudl/systemmodel.#SystemModel`. This includes previously omitted repair checks
such as Git, nous, `rules.cue`, and every future embedded built-in. Repository
initialization also creates the `schema/models` path used by `pudl model new`.

Selected non-secret resource handles in the built-in AWS, Git, Kubernetes,
filesystem, and artifact schemas now opt into `@pudl(binding=plain)`. Fields not
explicitly annotated remain unavailable to cross-model projection. Schema-path
lookup now follows optional CUE fields as well as required ones, so an explicitly
authorized optional handle can be projected when the observed record contains it.

## Compatibility and safety

- Standalone runs still never start a producer implicitly.
- `pudl run-set` remains a closed operator-named set and observe-only by default.
- Sealed outputs remain converge-only. Single-model sealed convergence works;
  sealed run-sets fail closed before approval until `pudl-olm` is resolved.
- PUDL-generated mu targets retain `sealed_routing: "strict"`, but mu v0.3.3's
  JSON plan does not expose the action claims required by PUDL's exact-plan
  validator.
- Init repair retains the existing install semantics while removing the drift-
  prone file checklist.

## Verification

- Focused importer, repo-init, command, and CUE validation tests.
- Generated skill synchronization check.
- Full Go test, CGO-disabled test, vet, build, and repository lint gates.
- Mu CUE validation, guide rendering, full test task, and repository verifier.
