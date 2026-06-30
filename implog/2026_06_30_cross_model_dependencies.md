# 2026-06-30 — Cross-model data dependencies (Phase 1) + query UX

Built the cross-model dependency feature from `docs/cross-model-dependencies.md`
(revised first through a three-lens adversarial review: usability, ubiquitous
language across pudl/mu, practicality). Phase 1 is the declared-dependency path:
a model names other models it depends on; pudl records those edges as bitemporal
facts and ships recursive Datalog rules to reason over them. Validated
end-to-end on a local **k3d** cluster against real k8s convergence.

## What shipped

### Schema
- `#SystemModel.depends_on?: [...string]` — NAMES of other `#SystemModel`
  instances (`internal/systemmodel/schema.cue`), added to `tracked_fields`.
  Decoded as `SystemModel.DependsOn []string` (`internal/systemmodel/systemmodel.go`).
  Model names, not value references (ordering/impact only — not interpolation,
  not Datalog rule refs; that is the separate `relations?` field).

### Fact reconciliation (idempotent, with retraction)
- `cmd/run_depends.go` — `reconcileModelDependencies(m)` runs inside `pudl run`
  (after `recordModelInstance`). It **diffs** the declared `depends_on` against
  the currently-valid `model_depends_on` facts for this model and:
  - declared & not current → `AddFact model_depends_on {from,to}`
  - current & not declared → `InvalidateFact` (valid-time end)
  - declared & already valid → no-op
  This avoids per-run fact churn (a blind `AddFact` would mint a new fact every
  run, since `valid_start` defaults to now → new content-addressed ID) and keeps
  blast-radius answers truthful when an author removes a dependency.
  Deps are canonicalized via `resolveModel`; an unresolved dep still records the
  edge (forward reference) but emits a warning.
- Pure diff extracted as `dependencyDiff(declared, current)` for unit testing.

### Built-in rules (`internal/importer/bootstrap/pudl/rules/convergence.cue`)
Non-definition top-level rules (so `ParseRules` picks them up), registered in
`bootstrapChecks` (`internal/importer/cue_schemas.go`) so existing workspaces
receive them, not just fresh `pudl init`:
- `depends_transitive(from,to)` — transitive closure (base reads
  `model_depends_on` directly; recursive step is the fixpoint join)
- `impacted_by(changed,impacted)` — reverse / blast radius
- `cyclic(model)` — a model transitively depending on itself

Arg-key contract (load-bearing across facts, rule bodies, CLI constraints):
`model_depends_on`/`depends_transitive` use `from`/`to`; `impacted_by` uses
`changed`/`impacted`; `cyclic` uses `model`.

### Query UX (`cmd/query.go`, `cmd/query_helpers.go`)
- `pudl query --list` — enumerate derived rule-head relations (with arg keys) +
  stored EDB fact relations. Closes the discoverability gap: rule heads never
  appear in shell completion (which lists only fact-table relations).
- `pudl query --topo <relation>` — read a relation's `from`/`to` edges as a
  topological run order (dependencies first); errors on a cycle, pointing at
  `pudl query cyclic`.
- `queryCmd` now sets `SilenceUsage` for clean error output.

### Stale-input advisory (`cmd/run.go`, `cmd/run_depends.go`)
- `pudl run --check-upstream` — opt-in read-only warning when any transitive
  upstream model is `drifted`/`failed`. Honors the coverage caveat (silence =
  "no recorded stale upstream", not proof). Best-effort; never blocks a run.

### Datalog compiler fix (`internal/datalog/compile.go`)
Latent bug surfaced by `impacted_by`: a rule body of a single override
(derived/EDB) atom whose args are all first-occurrence variables produces no
WHERE conditions, and the compiler emitted a bare `WHERE` → SQLite
`incomplete input`. Now the WHERE clause is omitted when there is nothing to
filter. General fix; guards all such reverse-projection rules.

## Public API / CLI surface

- CUE: `#SystemModel.depends_on?: [...string]`.
- Facts: relation `model_depends_on` with args `{from,to}` (not reserved;
  emitted by `pudl run`, writable via `pudl facts add`).
- Datalog relations: `depends_transitive{from,to}`, `impacted_by{changed,impacted}`,
  `cyclic{model}` (shipped built-in rules).
- CLI: `pudl query --list`, `pudl query --topo <relation>`,
  `pudl run --check-upstream`.

## Validation (local k3d, real k8s convergence)

Two models in an isolated `HOME`, a minimal mu root (disk cache only — bypasses
the private OCI registry), kubeconfig from a throwaway k3d cluster:
- `network` (converges a Namespace) and `workloads` (converges an nginx
  Deployment; `depends_on: ["network"]`).
- **convergence**: both `drift → apply → re-observe → clean` (1 iter each);
  cluster showed the namespace Active + the deployment created.
- **cross-model**: `model_depends_on(workloads→network)` emitted from a real run;
  `impacted_by changed=network → workloads`; `--topo → network, workloads`.
- **idempotency**: re-converge → drift ∅, 0 iters, still exactly 1 edge fact.
- **stale-upstream**: deleted the namespace → `network` drifted → `workloads
  --check-upstream` warned correctly.
- **retraction**: removed `depends_on` → next run invalidated the edge → query
  returns no results.

Unit tests: `cmd/run_depends_test.go` (dependencyDiff idempotent/add/invalidate/
both; edgeArgs), `internal/datalog/convergence_rules_test.go` (transitive,
blast radius, cycle detection), `internal/importer/convergence_bootstrap_test.go`
(shipped `.cue` parses + installs). `CGO_ENABLED=0 go test ./...` green.

## Not built (deferred, as designed)

- **Phase 2** — derive `model_depends_on` from `desired`-resource identities
  matching produced catalog rows. Needs a new EDB projection of desired-entry
  identities (the `catalog_entry` EDB is join-only); not the free,
  rules-unchanged extension an early draft implied.
- **Run-time-only coverage gap** — edges exist only for models that have been
  run; a `pudl model` discovery pass could emit edges from declared schema
  without a full run.
- **Downstream re-running** — explicitly NOT pudl (charter: pudl declares, mu
  executes). The mu DAG / an external scheduler consumes the relation.
- **Value threading (`${vpc.id}`)** — the ewe-converge item (mu §7).
