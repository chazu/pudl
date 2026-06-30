# `pudl run` / #SystemModel — build status & handoff

Implementation status of the V1 convergence build (the design lives in the **mu**
repo: `mu/docs/design/system-models/` — `V1-BUILD-SPEC.md` is canonical, `issue-ledger.md`
is the rationale). This doc is the **pudl-side build state**: what's built, what's
validated, what's next.

Branch: work merged to `pudl/main`. Code lives in `cmd/run*.go` +
`internal/systemmodel/`.

## What's built (and on main)

`pudl run <model>` runs a `#SystemModel` *instance* through the ACUTE phases.

| piece | file | status |
|-------|------|--------|
| `#SystemModel` schema + loader | `internal/systemmodel/` | ✅ schema.cue (pudl-owned, embedded); LoadModel/LoadModelFile |
| CLI contract (`--converge` gate, `--only`, `--dry-run`, `--max-iters`, `--from-catalog`, `--mu-root`) | `cmd/run.go` | ✅ + validateRunFlags |
| **populate — plugin-observe** (inventory observe → ingest) | `cmd/run_populate.go` | ✅ project-embedded (see below); live-tested vs k8s |
| **populate — ewe** (#EweTarget: render ewe target → `mu build` → wrap outputs → ingest) | `cmd/run_populate.go` (`runEwePopulate`) | ✅ e2e-validated vs a local HTTP fixture (mu v0.3.1): `pudl run` → 2 records ingested, incl. a **sealed env secret** revealed in-sink for an auth-required endpoint (via a `command`-based `envsecret` plugin declared in the model). |
| **drift — differential** (k8s: desired→sources, plugin diffs) | `cmd/run_drift.go` | ✅ live-tested vs k8s cluster |
| **drift — inventory** (host: set-diff desired vs catalog records) | `cmd/run_inventory.go` | ✅ via `--from-catalog`; validated vs real catalog (canned records) |
| **converge loop** (drift→apply→re-observe; clean/cap/exec_err/dry-run) | `cmd/run_converge.go` | ✅ dry-run live-tested; real apply wired, not auto-run |
| **checks** (Datalog relations over catalog, severity-gated) | `cmd/run_checks.go` | ✅ live-tested |
| **report** (RunReport → markdown / `--json`) | `cmd/run_report.go` | ✅ both renderings |

## Key architecture decisions (all grounded against real mu/pudl source)

- **Project-embedded (not standalone synthesis):** `pudl run` runs *within* a mu
  project — generates a per-package `mu.cue` in a **non-hidden temp subdir** under
  the mu project root (`--mu-root` or discovered), so mu merges it
  (`mergeSubdirConfigs` skips hidden dirs) and inherits the project's toolchains
  (e.g. `bb` for babashka plugins) + cache. The model carries plugin **logic**; the
  mu project provides **runtime**. (mu reads `mu.cue` only — the design's
  "mu.json → mu build" was wrong.)
- **Model-level `plugins:` block** (mirrors mu's `#PluginDef`): models are
  self-contained; pudl passes the block straight into the generated mu.cue. Plugin
  `script` paths emitted **absolute** (mu resolves a per-package source relative to
  the subdir; absolute just works for both observe and build).
- **Two drift styles:** *differential* (plugin diffs given desired-as-sources, e.g.
  k8s `observe`) vs *inventory* (plugin dumps records → catalog → pudl set-diffs,
  e.g. host). They are separate code paths.
- **`_schema` is a CUE hidden field** → `Decode` drops it; `decodeDesired` re-extracts
  desired with `cue.Hidden(true)` so identity survives.
- Inventory identity = `_schema` + first present key (`name|path|id`) — V1 heuristic;
  schema-driven `identity_fields` (from the inference graph) is a follow-up.

## How it's validated (and the environment limits)

- **k8s differential**: live cluster reachable → drift + converge **dry-run** proven
  end-to-end (real `mu observe`/`mu build --plan`).
- **inventory**: `HOME`-isolated catalog seeded with **canned host records** via
  `pudl mu ingest-observe`, then `pudl run … --from-catalog` → correct missing/changed/clean.
- **ewe-populate (built 2026-06-22):** the full chain works end-to-end under the
  real binaries — `pudl run` → render ewe target → `mu build` (executeEwe) →
  #HttpAll fetch + in-sink secret reveal → records file → wrap → ingest. Released
  as **ewe v0.1.0 / mu v0.3.0 / pudl v0.3.0**. Validated against a local HTTP
  fixture (no external infra). See "What's built" above.
- **Environment limits (why other paths aren't e2e-validated here):** the host
  (odroid `192.168.1.104`) SSH is **unreachable**; **no local inventory observer**
  (docker not running; aws/terraform need cloud). Don't build paths you can't
  validate — mock at the data layer (canned records / local HTTP fixture) instead.

## Not built (the frontier)

- **host.plan** — example 1's converge plugin: complete the `host` plugin's stub
  `plan` op (`mu/plugins/host/main.go:71`); spec: `mu/.../host-converge-spec.md`.
- ~~**Real converge apply**~~ — ✅ **VALIDATED LIVE (2026-06-30)** against a real k8s
  cluster (throwaway ns + nginx Deployment). The full declarative-apply loop runs
  end-to-end: drift → `kubectl apply --server-side` → re-observe → `clean`, idempotent,
  and drift→reapply on a deleted resource. Still **not** auto-run (operator opts in via
  `--converge`). See `implog/2026_06_30_k8s_convergence_live_validation.md`. Two real
  bugs found + fixed in the process: run-verdict status never persisted (the `//`-prefix
  key mismatch — `db86d13`) and `model validate` over-strict on differential `desired`
  (`3584746`); plus the install needed `pudl init --force` to pick up the `differential`
  schema field. ~~**Caveat:** ... promotion path remains unit-tested only.~~ ✅ **CLOSED
  (2026-06-30, `cb4db50`):** the converge loop now runs `mu build --emit-manifest` and
  ingests the manifest (`--model`-tagged), so each apply writes a `converging` row and
  the loop's re-observe ∅ promotes it to `clean` via `PromoteConvergingToCleanByModel`.
  Validated live: `pudl status` shows both the model row and the apply-action target
  (`models/<m>:drift:apply`) clean. Promotion trigger generalized to fire off a clean
  converge (not just observe-only drift). See
  `implog/2026_06_30_converge_ingest_wiring.md`.
- **`cloudflare-dns`** — post-V1, deliberately deferred (per the DNS disposition).
- ~~**Catalog status persistence**~~ — ✅ **DONE (2026-06-29).** The run verdict is
  written to the catalog status (`persistRunStatus`/`runVerdict` → `UpdateStatus`);
  `ingest-manifest` writes `converging` on apply (the in-flight state). The terminal
  in-sync status was collapsed to a single value: **`clean`** = drift == ∅ (verified
  by the drift re-check), written whether the model is observe-only or was just
  converged. The redundant `converged` status was removed (it and `clean` named the
  same state); the status vocabulary is now `unknown | drifted | converging | clean |
  failed`. `clean` is only ever written off an actual ∅ observation. A clean drift
  re-check also promotes this model's per-resource `converging` rows (from
  `ingest-manifest`) to `clean` — `promoteConvergingResources`. Exact path:
  `ingest-manifest --model <name>` tags rows (`tags.model`) and
  `CatalogDB.PromoteConvergingToCleanByModel` flips them regardless of action-target
  naming (k8s `Kind/name` included); falls back to a desired-resource-name heuristic for
  manifests ingested without `--model`. (End-to-end k8s validation still pending infra.)
- ~~**run.go dispatch**~~ — ✅ **DONE (2026-06-29).** Drift mode is auto-detected by
  observer style: EweTarget populate → inventory; `#PluginObserve` uses its
  `differential` field (default true → differential live observe; false → inventory
  set-diff from the catalog). `--from-catalog` remains an explicit override
  (`useInventoryDrift` / `SystemModel.DifferentialDrift`). A plain `pudl run` now
  routes inventory observers correctly without the flag.
- ~~**cross-model data dependencies (Phase 1)**~~ — ✅ **DONE (2026-06-30).**
  `#SystemModel.depends_on?: [...string]` (model names) → `pudl run` reconciles
  `model_depends_on(from,to)` facts (add new / invalidate removed — idempotent,
  no churn) → built-in recursive rules `depends_transitive` / `impacted_by` /
  `cyclic` (`bootstrap/pudl/rules/convergence.cue`) queryable via `pudl query`.
  UX: `pudl query --list` (relation/arg-key discovery), `pudl query --topo`
  (run order, errors on cycle), opt-in `pudl run --check-upstream` (stale-input
  advisory). Fixed a latent datalog compiler bug (bare `WHERE` on a no-condition
  body). **Validated end-to-end on a local k3d cluster:** real k8s convergence
  (Namespace + Deployment, drift→apply→clean) across two models with a real
  `depends_on` edge; cross-model queries, idempotency, retraction, and the
  stale-upstream warning all confirmed. Design + arg-key contract:
  [`docs/cross-model-dependencies.md`](cross-model-dependencies.md); see
  `implog/2026_06_30_cross_model_dependencies.md`. **Phase 2** (derived deps via
  a desired-identity EDB projection) remains future.

## Good next steps (self-contained, validatable here)

1. ✅ **Catalog status persistence** (§8) — DONE 2026-06-29 (see frontier above).
2. ✅ **Schema-driven identity** for inventory drift — DONE 2026-06-29. `recordIdentity`
   (`cmd/run_inventory.go`) now keys records by the schema's declared `identity_fields`
   (from the inference graph via `schemaIdentityResolver`), falling back to the
   name|path|id heuristic only when a schema declares none or a field is absent.
3. ✅ **Cross-model data dependencies — Phase 1** — DONE 2026-06-30 (see frontier
   above). Built + validated on k3d. Next here: **Phase 2** (derive
   `model_depends_on` from `desired`-resource identities matching produced catalog
   rows — needs a new EDB projection of desired identities, not free), and
   optionally a `pudl model` discovery pass that emits edges without a full run
   (closes the run-time-only coverage gap).
4. **Validate `--only` subset converge** — never exercised on real infra; small.
5. **host.plan** — complete the `host` plugin's stub plan op for example 1's
   converge arm (`mu/.../host-converge-spec.md`); needs the odroid reachable.

### Resolved this session (2026-06-30)
- ✅ **Cross-model data dependencies Phase 1** built + validated end-to-end on
  k3d (`depends_on` → `model_depends_on` → `depends_transitive`/`impacted_by`/
  `cyclic`; `query --list`/`--topo`; `run --check-upstream`). Latent datalog
  compiler bug fixed (bare `WHERE`).
- ✅ Real converge apply **validated live on k8s** + the per-resource
  `converging`→`clean` lifecycle wired (`cb4db50`) and validated.
- ✅ Finding A (converge needs explicit `kubeconfig` — mu's hermetic env)
  documented in the `#PluginPlan` schema + `cli-reference` (`3089cdd`).
- ✅ `pudl init --force` skill-clobber fixed (embedded skill made canonical,
  `62998c2`).
- ✅ Catalog `definition` column renamed → `target` (`576e0cd`).

## Repro / smoke commands

```sh
# inventory drift, isolated catalog, canned records (no host needed):
H=/tmp/pt; rm -rf $H; mkdir -p $H
HOME=$H pudl mu ingest-observe --path canned_host.json     # records with _schema + name
HOME=$H pudl run <inst> --file model.cue --from-catalog [--json]

# k8s differential drift (needs a reachable cluster):
pudl run <inst> --file k8s.cue --mu-root /path/to/mu        # bare = drift; --converge --dry-run = plan
```
