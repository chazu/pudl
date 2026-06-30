# 2026-06-30 — Declarative-apply convergence: first live k8s validation + fixes

Ran the `#SystemModel` convergence loop against a **real k8s cluster**
(`loosh-loosh-platform`, prod) for the first time — the path the V1 build spec
called "wired, not auto-run / pending infra". Confined to a throwaway
`pudl-converge-test` namespace with a single nginx Deployment as `desired`,
deleted afterward.

## What was validated (✅ works end-to-end)

The declarative-apply class — pudl renders `desired` to a manifest, the `k8s`
plugin runs `kubectl apply --server-side`, kubectl reconciles, pudl re-observes:

- **drift → apply → re-observe → clean**: drift saw `Deployment/nginx (missing)`,
  applied, re-observed ∅, outcome `clean`, 1 iteration. nginx came up 1/1 Running.
- **idempotent**: re-run → drift ∅ immediately, 0 applies.
- **drift → reapply**: deleted the deployment, `--converge` re-detected + reapplied → clean.
- **observe-only differential drift**: bare `pudl run` (after the D fix) detects the
  deleted deployment as `drifted` and persists it.

Repro (model was a throwaway in `~/.pudl/schema/models/`, since removed):
`pudl run k8s-converge-test --converge --mu-root ~/dev/go/mu`.

## Findings (4) — the value of real infra

- **A — apply needs `kubeconfig:` passed explicitly.** `mu build` actions run with a
  *minimal env* (no `HOME`; mu's executor `buildEnv`), so `kubectl` can't find
  `~/.kube/config` — apply failed `context "…" does not exist` until the converge
  `input.kubeconfig` was set. Observe is unaffected (runs in the plugin's `bb`
  subprocess with full env). Not documented; bites every first real apply. **Open.**
- **B — run verdicts never persisted (`//`-prefix key mismatch).** ✅ **FIXED**
  (`db86d13`). `persistRunStatus` / `model_list` keyed on `modelTarget` =
  `//models/<name>`, but the observe ingester stores the instance row with `//`
  stripped (`models/<name>`). `UpdateStatus`'s WHERE matched nothing → STATUS stayed
  `unknown`/`-` regardless of outcome. Added `modelDefinition(name)` (stripped form)
  and routed the write + both reads through it. Verified: STATUS now shows `clean`
  after converge, `drifted` after an observe of a deleted resource.
- **C — `pudl model validate` over-strict.** ✅ **FIXED** (`3584746`). It demanded a
  quoted `_schema` tag on every `desired` entry, but that only matters for
  inventory models (set-diff by `_schema`). Differential models (k8s) route raw
  manifests to the plugin — no `_schema`. Gated on `!m.DifferentialDrift()`.
- **D — installed `~/.pudl/schema` copy of `#SystemModel` was stale.** Missing
  `differential`, still carried the removed `vault`. `resolveModel` decodes against
  the *repo* copy, so `Populate.Differential` decoded `false` → bare `pudl run`
  misrouted k8s models to **inventory** drift (converge unaffected — it doesn't use
  `differential`). **Fixed by `pudl init --force`** (the schema-sync mechanism;
  re-installs the embedded schema). No code change.

(Also: CUE silently ignores `*_test.cue` files — a model named `*_test.cue` won't load.)

## Still NOT validated against real infra

The **`ingest-manifest --model` → `converging` → `PromoteConvergingToCleanByModel`
→ `clean`** path is still unexercised. `pudl run --converge` applies via `mu build`
directly and writes `clean` through `runVerdict` — it never ingests manifests or
sets `converging`, so the exact-promotion path (and its action-target naming
assumption) remains unit-tested only. A real validation needs a converge that goes
through `ingest-manifest --model`.

## Public surface touched

- `cmd/run_resolve.go`: new `modelDefinition(name)` (the `//`-stripped catalog
  definition key for status reads/writes).
- `cmd/run.go`, `cmd/model_list.go`: status sites route through `modelDefinition`.
- `cmd/model_validate.go`: `_schema` requirement gated on `!m.DifferentialDrift()`.
