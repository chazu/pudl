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
    populators/          # populator programs for #EweTarget models
  data/
    sqlite/catalog.db    # SQLite catalog (modernc, pure Go)
    .runs/               # run manifests
    .drift/              # drift reports
```

## Common Commands

### Data pipeline
- `pudl import --path <file>` — import JSON/YAML/CSV/NDJSON (schema inferred unless `--schema` given; `--path` takes globs and `-` for stdin)
- `pudl list` — list catalog entries (default shows all; `--artifacts` = run outputs)
- `pudl show <id>` / `pudl export <id>` / `pudl delete <id>`
- `pudl validate` — validate catalog data against assigned schemas

### Schema
- `pudl schema list|show <name>` — browse schemas
- `pudl schema new <name>` — generate a schema from data
- `pudl schema add` — register a definition (e.g. a `#SystemModel`)
- `pudl module add <module@version>` — add CUE module deps

### Definitions & facts
- `pudl definition list|show <name>|validate` — named instances of schemas
- `pudl facts` — query the bitemporal fact store
- `pudl query` — derived facts via Datalog rules; `pudl rule` manages rules
- `pudl pull <scope|entity>` — retrieve all related facts

### #SystemModel loop
- `pudl run <model>` — run a registered `#SystemModel` (OBSERVE-ONLY by default)
- `pudl run <model> --converge` — close drift (mutates the target via mu)
- `pudl run <model> --from-catalog` — drift over ingested records, no live observe
- `pudl model list` — list registered `#SystemModel` definitions + last-run status
- `pudl model show <model>` — show a model's populate/converge/desired/checks
- `pudl model validate <model>` — structural validation without running
- `pudl model populator add ...` — manage populator programs for `#EweTarget`
- `pudl status` — convergence status of definitions
- `pudl drift check <definition>` — declared vs live state

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
  `drift==∅ -> converged | iteration cap -> failed | else converge -> execute -> re-observe`
  (`--max-iters`, `--dry-run`, `--only <defs>`).
- Each run records the model instance in the catalog (identity = name) so it's
  inventoriable via `pudl list` / `pudl query`.

### mu bridge

mu writes its results back into the pudl catalog via:
- `pudl mu ingest-observe` — ingest observe results (`entry_type=observe`)
- `pudl mu ingest-manifest` — ingest a build manifest (`entry_type=manifest`,
  per-action `manifest-action`)
- `pudl mu export-actions` — export drift reports as a `mu.json` config

These `entry_type` values are what `pudl list --artifacts` surfaces (run
outputs), vs ingested/observed data.
