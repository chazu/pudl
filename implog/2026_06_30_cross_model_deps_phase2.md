# 2026-06-30 — Cross-model deps: Phase 2 (derived) + Phase 1 leftovers closed

Follow-on to `2026_06_30_cross_model_dependencies.md`. Closes the two Phase-1
leftovers and builds Phase 2 (derived dependencies), validated on local k3d and
against a Docker container acting as a fake remote host.

## Phase 1 leftovers — closed

### `pudl model deps` — no-run discovery pass (coverage gap)
`cmd/model_deps.go`. Reconciles **every registered model's** declared
`depends_on` into `model_depends_on` facts **without running them**. Previously
edges existed only for models that had been `pudl run`, so `impacted_by` was
blind to never-run models (an empty result meant "no *recorded* dependents," not
"none"). After `pudl model deps`, the graph reflects the whole declared schema.
Prints the graph grouped by model, annotated declared/derived; `--json` supported.

### Query completion lists derived relations (discoverability)
`cmd/completion.go` `completeRelations` now folds **derived rule-head relations**
(loaded via `loadQueryRules`) and **built-in EDB relations**
(`database.ReservedRelations()`) into the candidate set, deduped against stored
fact relations. `depends_transitive` / `impacted_by` / `cyclic` now complete even
before any fact exists. (`pudl query --list`, shipped earlier, remains the
explicit listing.)

## Phase 2 — derived dependencies (`pudl model deps --derive`)

Derives `model_depends_on(from:B, to:A)` when a value in B's `desired` references
an identity A produces — no manual `depends_on`.

**Why Go-side, not a Datalog join.** The original design imagined a Datalog join
over a new EDB projection of desired identities. The substrate makes that
impractical: `desired` is **not** SQL-queryable (it lives in the in-memory model
/ the stored record file, not a catalog column), and `tags.model` is set only by
the converge path. So derivation runs in Go over resolved models and emits the
**same** `model_depends_on` relation under a separate `derived:` fact source —
the Phase-1 rules are unchanged, as the design required ("derivation produces the
same relation").

**The match (`cmd/model_derive.go`, all pure/unit-tested):**
- `producedIdentities(desired, identity)` = top-level identity values
  (`modelResourceDefs`: identity_fields or name|path|id) ∪ the k8s `metadata.name`
  (scoped to a metadata sub-map — container/port/volume names are NOT treated as
  produced identities).
- `referencedValues(desired)` = string leaves of the desired entries, skipping
  structural type tags (`kind`/`apiVersion`/`_schema`) so two models sharing a
  kind can't mint a spurious edge.
- `deriveDependencies(models, identity)`: B→A when `referencedValues(B)` minus
  B's own produced identities intersects `producedIdentities(A)`, A ≠ B, and B
  does not already **declare** A (declared wins — no duplicate).

It is value-based and therefore **heuristic** (a coincidental string equality can
over-match), so it is **opt-in** (`--derive`), **separately sourced** (auditable;
never corrupts the declared graph), and skips already-declared edges.

## Reconcile refactor (enables coexistence)

`cmd/run_depends.go`: extracted `reconcileEdges(db, from, source, wanted)` which
scopes the current-edge query by fact **Source**. Declared edges use source
`model:<name>`, derived edges `derived:<name>`. The two graphs reconcile
independently and never clobber each other; both stay idempotent (no per-run
churn). `declaredDepsOf` extracted for reuse by the run path and the discovery
pass. A derived edge that later becomes declared flips cleanly (the derive pass
invalidates its `derived:` edge; the declared pass adds the `model:` edge).

## Public API / CLI surface (new this change)

- `pudl model deps` — reconcile + print the declared graph for all models (no run).
- `pudl model deps --derive` — also compute + reconcile Phase-2 derived edges.
- `pudl model deps --json` — machine-readable `{edges:[{from,to,source}], warnings}`.
- Shell completion for `pudl query <relation>` now includes derived rule heads + EDB relations.

## Validation

- **Unit** (`cmd/model_derive_test.go`): producedIdentities (nested k8s
  metadata.name + top-level), referencedValues, deriveDependencies (namespace
  reference derives the edge / skips a declared edge / no self-edge). Full suite
  `CGO_ENABLED=0 go test ./...` green — no regressions.
- **k3d (Phase 2 on real convergence):** `network` (Namespace) + `workloads`
  (Deployment) where **workloads declares NO `depends_on`**. `pudl model deps
  --derive` derived `workloads → network [derived]` from the Deployment's
  `metadata.namespace` referencing `network`'s Namespace; both models converged
  on the cluster; the derived edge survived the real run reconcile (source
  separation). `impacted_by` / `--topo` reflected it.
- **Docker-as-remote-host (inventory class, no remote infra):** an alpine
  container observed via `docker exec apk info` → mu-observe-shaped records →
  `pudl mu ingest-observe` → two inventory models (`base-host`, `app-host
  depends_on base-host`). Validated: inventory set-diff drift (`jq` missing
  detected), cross-model deps on the inventory class (`pudl model deps`,
  `impacted_by`, `--topo`), and the `--check-upstream` stale-upstream warning
  firing when `base-host` drifted. Demonstrates the feature is convergence-class
  agnostic. Test harness: scratch scripts (not committed).

## Still deferred

- **Deletion-safety warning** — `pudl delete` is generic catalog-entry deletion
  (by proquint), not model-aware; a model-aware blast-radius warn is a separate
  small follow-up.
- **Datalog-side derivation** — would require exposing desired identities + tags
  as EDB columns; the Go-side derivation supersedes the need.
- **Downstream re-running / value threading** — out of scope by charter / the
  ewe-converge item, as before.
