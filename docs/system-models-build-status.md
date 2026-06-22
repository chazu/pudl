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
| **populate** (inventory observe → ingest) | `cmd/run_populate.go` | ✅ project-embedded (see below); live-tested vs k8s |
| **drift — differential** (k8s: desired→sources, plugin diffs) | `cmd/run_drift.go` | ✅ live-tested vs k8s cluster |
| **drift — inventory** (host: set-diff desired vs catalog records) | `cmd/run_inventory.go` | ✅ via `--from-catalog`; validated vs real catalog (canned records) |
| **converge loop** (drift→apply→re-observe; converged/cap/exec_err/dry-run) | `cmd/run_converge.go` | ✅ dry-run live-tested; real apply wired, not auto-run |
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
- **Environment limits (why other paths aren't e2e-validated here):** the host
  (odroid `192.168.1.104`) SSH is **unreachable**; **no local inventory observer**
  (docker not running; aws/terraform need cloud); **ewe runtime unbuilt**. Don't
  build paths you can't validate — mock at the data layer (canned records) instead.

## Not built (the frontier)

- **ewe-populate** — custom HTTP-fetch observers (examples 3/4/5: TLS/DNS/GitLab).
  The big one: ~3-repo build (ewe engine fix + `#Secret` + `#HttpAll` + the `ewe`
  body-kind in mu + the small pudl ingest seam). Designed in `mu/docs/design/system-models/ewe-*-spec.md`
  + `ewe-populate-spec.md`. **Only needed where no plugin exists** — plugin-based
  observers (k8s/host/aws…) need zero ewe. Do as a dedicated multi-session push.
- **host.plan** — example 1's converge plugin: complete the `host` plugin's stub
  `plan` op (`mu/plugins/host/main.go:71`); spec: `mu/.../host-converge-spec.md`.
- **Real converge apply** — wired (`mu build`), not auto-run (mutates live systems).
- **`cloudflare-dns`** — post-V1, deliberately deferred (per the DNS disposition).
- **Catalog status persistence** — write the run verdict (drifted/converged/failed)
  to the catalog status (`UpdateStatus`); narrow `ingest-manifest` `converged→converging`
  (build-spec §5/§8). Self-contained + validatable; good next step.
- **run.go dispatch** — inventory-vs-differential is currently explicit
  (`--from-catalog` = inventory). Auto-detect by observer style is a follow-up.

## Good next steps (self-contained, validatable here)

1. **Catalog status persistence** (§8) — write/read-back the run verdict; no infra.
2. **Schema-driven identity** for inventory drift — replace the name|path|id
   heuristic with `identity_fields` from the inference graph; validatable vs catalog.
3. **ewe-populate** — the real frontier; needs dedicated effort + spans repos.

## Repro / smoke commands

```sh
# inventory drift, isolated catalog, canned records (no host needed):
H=/tmp/pt; rm -rf $H; mkdir -p $H
HOME=$H pudl mu ingest-observe --path canned_host.json     # records with _schema + name
HOME=$H pudl run <inst> --file model.cue --from-catalog [--json]

# k8s differential drift (needs a reachable cluster):
pudl run <inst> --file k8s.cue --mu-root /path/to/mu        # bare = drift; --converge --dry-run = plan
```
