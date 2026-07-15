---
name: pudl-core
description: Using the pudl CLI — data lake import/query, CUE schemas, the bitemporal fact store, and the #SystemModel observe/converge loop that drives mu. Use when working with pudl commands, schemas, facts, or running/converging system models.
---

# PUDL Core

CLI for an infrastructure data lake: import data, infer/validate CUE schemas,
store bitemporal facts, and run `#SystemModel` instances that observe and
converge real infrastructure through the **mu** execution layer.

There is no execution layer inside pudl — models, methods and workflows were
extracted to mu. pudl declares desired/observed state; mu mutates the world.

## Repository Layout

```
~/.pudl/                 # global (project .pudl/ shadows it)
  config.yaml            # workspace configuration
  schema/                # CUE schema repository (git-tracked)
    cue.mod/             # CUE module metadata
    pudl/                # built-in + local schema defs (incl. #SystemModel)
    pudl/rules/          # Datalog rules
    populators/          # populator programs for #EweTarget models
  data/
    sqlite/catalog.db    # SQLite catalog (modernc, pure Go)
```

## Common Commands

### Data pipeline
- `pudl import --path <file>` — import JSON/YAML/CSV/NDJSON (schema inferred unless `--schema` given; typed envelopes preserve schema metadata; `--path` takes globs and `-` for stdin)
- `pudl list` — list catalog entries (default shows all; `--artifacts` = run outputs)
- `pudl show <id>` / `pudl export <id>` / `pudl delete <id>`
- `pudl validate` — validate catalog data against assigned schemas

### Schema
- `pudl schema list|show <name>` — browse schemas
- `pudl schema new <name>` — generate a schema from data
- `pudl schema add` — register a definition (e.g. a `#SystemModel`)
- `pudl module add <module@version>` — add CUE module deps

### Facts
- `pudl facts` — query the bitemporal fact store
- `pudl query <relation> [key=value ...]` — derived facts via Datalog rules
  (positional `key=value` constraints, not `--where`); `pudl rule` manages rules
- `pudl query --list` — list queryable relations (rule heads + EDB facts) and their arg keys
- `pudl query --topo <relation>` — read a relation's `from`/`to` edges as a topological order (errors on a cycle)
- `pudl pull <scope|entity>` — retrieve all related facts

### #SystemModel loop
- `pudl model list` — list registered `#SystemModel` definitions + last-run status
- `pudl model show <model>` — show a model's populate/converge/desired/checks
- `pudl model validate <model>` — structural validation without running
- `pudl run <model>` — run a registered `#SystemModel` (OBSERVE-ONLY by default)
- `pudl run <model> --converge` — close drift (mutates the target via mu)
- `pudl run <model> --from-catalog` — explicitly replay ingested records for inventory drift; a normal inventory run populates and compares its own current snapshot
- `pudl run <model> --check-upstream` — warn if any transitive upstream (depends_on) model is `drifted`/`failed`
- `pudl model deps` — reconcile + show the cross-model dependency graph (no run needed)
- `pudl model deps --derive` — also derive edges from desired↔produced identity matching
- `pudl model populator add ...` — manage populator programs for `#EweTarget`
- `pudl status [target]` — recorded convergence status by catalog target (a run records its verdict)

### Utilities
- `pudl init` / `pudl doctor` / `pudl config show` / `pudl version`
- `pudl guide` / `pudl prime` — agent-facing usage reference

## How pudl drives mu (the #SystemModel loop)

`pudl run <model>` resolves a `#SystemModel` definition (project `.pudl/schema`
wins over global `~/.pudl/schema`; register with `pudl schema add`) and runs the
ACUTE cycle:

```
1. populate -> ingest   (Accumulate observed state into the catalog)
2. drift                (Unify desired vs observed)
3. checks               (flag violations)
4. report
```

- **Populate arm**: either a plugin (live observe inside an existing mu project,
  discovered via `mu.cue` from the model dir, override with `--mu-root`) or an
  `#EweTarget` whose populator self-stages its own temp mu project.
- **Default is observe-only** — no mutation. `--converge` opts into the loop:
  `drift==∅ -> clean | iteration cap -> failed | else converge -> execute -> re-observe`
  (`--max-iters`, `--dry-run`, `--only <selectors>`). `--only` is a converge-only
  preflight filter: selectors match desired resource names or schema paths and
  include transitive `depends_on` resources; unknown selectors fail before side effects.
- **Converge plugins run hermetically.** mu executes actions with a minimal
  environment (no inherited `HOME`), so a converge plugin that needs host
  credentials must receive them through the model's `converge.input` — e.g. the
  k8s plugin needs `input.kubeconfig: "/path/to/kubeconfig"` or it cannot find
  `~/.kube/config`.
- Each run records the model instance in the catalog (identity = name) so it's
  inventoriable via `pudl list` / `pudl query`.

### mu bridge

mu writes its results back into the pudl catalog via:
- `pudl mu ingest-observe` — ingest observe results (`entry_type=observe`)
- `pudl mu ingest-manifest` — ingest a build manifest (`entry_type=manifest`,
  per-action `manifest-action`); `--model <name>` tags rows so a later clean
  drift re-check promotes the model's `converging` resources to `clean`

`pudl run --converge` renders the model's `desired` state to sources and runs
`mu build --emit-manifest`; the mu plugin reconciles, and pudl ingests the
manifest (per-resource `converging` → `clean` on the re-observe). pudl computes
no domain ops.

These `entry_type` values are what `pudl list --artifacts` surfaces (run
outputs), vs ingested/observed data.

### Workspace schema precedence

Inside a repository workspace, PUDL searches `<repo>/.pudl/schema/` before the
global `~/.pudl/schema/`. The first matching definition wins, while unrelated
global schemas remain available. `pudl config` reports the configured global
path and effective search order. See `docs/workspace.md`.

## Cross-model dependencies

A model can depend on another model's output. Declare it with `depends_on` (a
list of model **names**) on the `#SystemModel`:

```cue
#Workloads: sm.#SystemModel & { name: "workloads", depends_on: ["network"], ... }
```

`pudl run` (and `pudl model deps`) reconcile declared deps into bitemporal
`model_depends_on(from,to)` facts. Built-in recursive Datalog rules reason over
them (query with positional `key=value`):

- `pudl query depends_transitive from=<m>` — what `<m>` depends on (transitively)
- `pudl query impacted_by changed=<m>` — blast radius: who depends on `<m>`
- `pudl query cyclic` — models in a dependency cycle (no valid run order)
- `pudl query --topo model_depends_on` — a topological run order (deps first)

`pudl model deps` records edges for **every** registered model without running
them (closes the gap where impact was blind to never-run models). `--derive`
adds Phase-2 derived edges: when a value in B's `desired` references an identity
A produces (e.g. B's Deployment names a Namespace A declares), B→A is derived
without a manual `depends_on` (heuristic, opt-in, separately sourced). pudl only
makes deps queryable — it does not re-run downstream models (that is mu's / a
scheduler's job). See `docs/cross-model-dependencies.md`.
