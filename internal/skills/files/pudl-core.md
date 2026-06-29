# PUDL Core

CLI tool for infrastructure data management and automation.

## Repository Layout

```
~/.pudl/
  config.yaml          # workspace configuration
  schema/              # CUE schema repository (git-tracked)
    cue.mod/           # CUE module metadata
    pudl/              # built-in + local schema defs (incl. #SystemModel)
    pudl/rules/        # Datalog rules
    populators/        # populator programs for #EweTarget models
  data/
    sqlite/catalog.db  # SQLite catalog
    .runs/             # run manifests
```

## Common Commands

### Data Pipeline
- `pudl import --path <file>` — Import JSON, YAML, CSV, or NDJSON
- `pudl import --path <file> --infer-schema` — Import with schema inference
- `pudl list` — List imported data entries
- `pudl show <id>` — Show entry details
- `pudl export <id>` — Export entry data
- `pudl validate <file>` — Validate data against schemas

### Schema Management
- `pudl schema list` — List schemas
- `pudl schema show <name>` — Show schema details
- `pudl schema new <name>` — Generate schema from data

### Models (`#SystemModel`)
- `pudl model list` — List registered models (+ last-run status)
- `pudl model show <name>` — Show populate/converge/desired/checks
- `pudl model validate <name>` — Structural validation without running

### Convergence (`pudl run`)
- `pudl run <name>` — Observe-only ACUTE loop for a `#SystemModel`: populate, drift, checks, report
- `pudl run <name> --converge` — Close drift: render the model's `desired` state to a sources file and run `mu build` (the mu plugin reconciles)
- `pudl status` — Read catalog convergence status (a model run records its verdict on `//models/<name>`)

### Utilities
- `pudl init` — Initialize workspace
- `pudl doctor` — Health checks
- `pudl config show` — Show configuration
