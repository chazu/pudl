# PUDL Core

CLI tool for infrastructure data management and automation.

## Repository Layout

```
~/.pudl/
  config.yaml          # workspace configuration
  schema/              # CUE schema repository (git-tracked)
    cue.mod/           # CUE module metadata
    pudl/              # local schema definitions
    models/            # model CUE files (schema + behavior)
    definitions/       # named instances of models
    methods/           # Glojure .clj method implementations
    extensions/models/ # user extension models
    examples/          # usage examples
  data/                # imported data + artifacts
    sqlite/catalog.db  # SQLite catalog
    .runs/             # workflow run manifests
    .drift/            # drift detection reports
  vaults/              # encrypted credential stores
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

### Models & Definitions
- `pudl model list` — List available models
- `pudl model show <name>` — Show model details
- `pudl model search <query>` — Search models by keyword
- `pudl model scaffold <name>` — Generate model boilerplate
- `pudl definition list` — List definitions
- `pudl definition show <name>` — Show definition details
- `pudl definition validate` — Validate all definitions

### Method Execution
- `pudl method run <definition> <method>` — Execute a method
- `pudl method run --dry-run <def> <method>` — Dry run (qualifications only)
- `pudl method list <definition>` — List methods for a definition

### Workflows
- `pudl workflow run <name>` — Execute a workflow DAG
- `pudl workflow list` — List workflows
- `pudl workflow validate <name>` — Validate workflow

### Infrastructure
- `pudl drift check <definition>` — Compare declared vs live state
- `pudl vault set <path> <value>` — Store a secret
- `pudl vault get <path>` — Retrieve a secret

### Utilities
- `pudl init` — Initialize workspace
- `pudl doctor` — Health checks
- `pudl config show` — Show configuration
