# Sealed run-set bridge completion

**Date:** 2026-08-05
**PUDL tracker:** `pudl-olm`
**Mu commits:** `50921c5`, `a334872`, `6e757b8`, integrated by `3d44291`

## Outcome

The real PUDL-to-mu sealed exact-plan path is executable. Mu plan schema v2
projects every execution-affecting action field, including sealed refs and
modes. Strict targets reject unused declarations, undeclared claims, ref/mode
changes, and ambiguous output writers during planning.

PUDL now activates phase-specific generated configuration. Read-only drift
observation excludes converge-owned sealed inputs and outputs, so whole-set
preflight can run before a producer creates its secret. Exact planning and apply
activate the full strict converge projection. The generated project remains
serialized by the existing per-mu-root lock.

The real repository smoke proves:

- producer-first sealed output storage and downstream input delivery;
- automatic exact-plan approval, process resume, and plan revalidation;
- no mutation or provider calls during pending approval or routing rejection;
- fail-closed unused, undeclared, and ambiguous action claims;
- no resolved value or raw writable-policy path in process output or the SQLite
  catalog. Complete provider destinations appear only in the intentional human
  approval display; JSON output and durable reports use fingerprints.

The final adversarial pass also closed approval-to-execution TOCTOU: every
apply revalidates the normalized approved graph, then mu compares the raw plan
digest before provider access and executes that same in-memory `PlanResult`.
Plan v2 commits resolved plugin identities, and mutable command plugins are
rejected on the guarded path.

The last redaction assertion exposed one additional gap: sealed evidence stored
the matched writable-policy pattern verbatim. Durable evidence now records its
SHA-256 fingerprint instead.

## Initialization inventory

No new product model belongs in `pudl repo init`. The kick-tires definitions are
repository test fixtures. Init and repair already derive their owned inventory
from every embedded bootstrap schema and separately install the single-sourced
`pudl/systemmodel.#SystemModel`; tests enumerate that complete inventory.

The existing `pudl/mu` built-in did need a content update. `#PlanOutput` now
matches plan schema v2 (plan digest, resolved plugin identities, and every
action execution field), while `#Target` includes strict routing and sealed
output modes. The embedded source and repository copy are synchronized, and
bootstrap validation tests exercise representative current plan and target
documents. A fresh `pudl repo init` was also verified to install these schemas,
the system-model schema, and the catalog beneath the repository `.pudl/` root.

## Adversarial closure and verification

Three independent reviews found and drove closure of the stale schema, the
approval-to-execution race, provider re-resolution, graph-eager secret reads,
durable error/action-ID redaction, and remaining live usage-text mismatches.
Regression coverage now includes stateful planner rejection before execution,
plugin-content identity changes, execution of the exact validated graph,
retention of the planned provider artifact, skipped provider reads for cancelled
actions, plan-v1/tamper rejection, and the real cross-process PUDL/mu smoke.

Verified with full Go tests and race tests in both repositories, vet/build,
scoped zero-new-issue golangci-lint, CUE validation, skill-copy validation,
fresh repository initialization, live `mu guide` output, and
`make test-kick-tires` against the in-tree mu v0.3.5 binary.
