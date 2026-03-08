# PUDL — Personal Unified Data Lake & Infrastructure Automation

PUDL is a CLI tool that combines a schema-validated personal data lake with declarative infrastructure automation. Import JSON, YAML, CSV, and NDJSON from AWS, Kubernetes, or any source — then define models, methods, and workflows to manage that infrastructure as code. PUDL detects formats, infers schemas, tracks provenance, deduplicates, orchestrates operations, and detects drift automatically.

Everything runs locally. One binary, SQLite catalog, CUE schemas, Glojure methods, all tracked in a local git repo.

## Quick Start

```bash
# Build and initialize
go build -o pudl .
./pudl init

# Import some data
pudl import --path aws-ec2-instances.json
pudl import --path k8s-pods.yaml

# Query your catalog
pudl list
pudl show mivof-duhij --raw

# Define infrastructure and run operations
pudl definition validate
pudl method run my-server restart --dry-run
pudl drift check my-server
pudl workflow run deploy-stack
```

## What Happens When You Import

```
pudl import --path data.json
    |
    +-- SHA256 content hash -> deduplicate (skip if already imported)
    +-- Detect format (json, yaml, csv, ndjson)
    +-- Detect if it's a collection wrapper (e.g. {"items": [...], "count": 5})
    +-- Infer schema via heuristics + CUE unification
    +-- Extract resource identity (stable ID across re-imports)
    +-- Store raw file in ~/.pudl/data/raw/YYYY/MM/DD/
    +-- Catalog in SQLite with full provenance metadata
```

Data is never rejected — if no specific schema matches, it falls back to the universal `pudl/core.#Item` catchall.

## What Happens When You Run a Method

```
pudl method run my-server restart
    |
    +-- Resolve definition "my-server" and its model
    +-- Look up "restart" method on the model
    +-- Run :before advice (pre-checks, validation)
    +-- Execute Glojure method body (produces an Effect)
    +-- Dispatch effect to the appropriate handler
    +-- Run :after advice (cleanup, notifications)
    +-- Store result as a versioned artifact
```

Methods return declarative Effect values describing what should happen, keeping logic pure and testable.

## Key Concepts

- **Content-addressed IDs**: Files are identified by SHA256 hash, displayed as pronounceable [proquint](https://arxiv.org/html/0901.4016) words like `mivof-duhij`
- **CUE schemas with `_pudl` metadata**: Schemas define both data shape and inference hints (identity fields, tracked fields, cascade priority)
- **Schema inference**: Heuristic scoring narrows candidates, then CUE unification validates matches — most specific schema wins
- **Resource identity**: Same logical resource tracked across re-imports via `resource_id` (schema + identity fields hash)
- **Collections**: NDJSON files and JSON API wrappers are automatically split into individual items with parent references
- **Models**: Compose schemas with behavior — a model pairs a CUE schema with methods, advice, and metadata
- **Definitions**: Named instances of models with concrete configuration values
- **Methods**: Glojure operations with lifecycle dispatch (before/after advice) that produce Effects
- **Workflows**: DAG-based orchestration of multiple method calls across definitions
- **Drift Detection**: Compare declared definition state against live infrastructure state using JSON deep diff
- **Vault**: Local encrypted credential management for secrets referenced by definitions
- **Effects**: Declarative descriptions of side effects (shell commands, HTTP calls, file writes) returned by methods

See [docs/concepts.md](docs/concepts.md) for a deeper explanation of these ideas.

## Commands

### Data Import & Catalog

| Command | Description |
|---------|-------------|
| `pudl init` | Initialize workspace (`~/.pudl/`) |
| `pudl import --path <file>` | Import data with automatic detection |
| `pudl list` | Query catalog (filter by `--schema`, `--origin`, `--format`, etc.) |
| `pudl show <id>` | Inspect an entry (`--raw`, `--metadata`, `--validation`) |
| `pudl delete <id>` | Remove entry (`--cascade` for collections) |
| `pudl schema list` | List schemas (`--package`, `--verbose`) |
| `pudl schema new --from <id>` | Generate CUE schema from imported data |
| `pudl schema add <name> <file>` | Add a schema to the repository |
| `pudl validate` | Validate data against schemas |
| `pudl doctor` | Health check |

### Models & Definitions

| Command | Description |
|---------|-------------|
| `pudl model list` | List models (`--category`, `--verbose`) |
| `pudl model show <name>` | Show model details |
| `pudl model search <query>` | Search models by keyword |
| `pudl model scaffold <name>` | Generate model boilerplate |
| `pudl definition list` | List definitions |
| `pudl definition show <name>` | Show definition details |
| `pudl definition validate` | Validate definitions against their models |
| `pudl definition graph` | Show dependency graph |
| `pudl repo validate` | Validate all schemas, models, definitions |

### Method Execution

| Command | Description |
|---------|-------------|
| `pudl method run <def> <method>` | Execute a method (`--dry-run`, `--skip-advice`, `--tag`) |
| `pudl method list <def>` | List available methods for a definition |

### Workflows

| Command | Description |
|---------|-------------|
| `pudl workflow run <name>` | Execute workflow DAG |
| `pudl workflow list` | List workflows |
| `pudl workflow show <name>` | Show workflow details |
| `pudl workflow validate <name>` | Validate workflow definition |
| `pudl workflow history <name>` | View run history |

### Drift Detection

| Command | Description |
|---------|-------------|
| `pudl drift check <def>` | Compare declared vs live state (`--all`, `--refresh`) |
| `pudl drift report <def>` | Show last drift report |

### Vault

| Command | Description |
|---------|-------------|
| `pudl vault get <path>` | Retrieve secret |
| `pudl vault set <path> <value>` | Store secret |
| `pudl vault list` | List stored paths |
| `pudl vault rotate-key` | Re-encrypt vault with new key |

### Data Search

| Command | Description |
|---------|-------------|
| `pudl data search` | Search artifacts by definition, method, tag |
| `pudl data latest <def> <method>` | Show most recent artifact |

See [docs/cli-reference.md](docs/cli-reference.md) for the full command reference.

## Writing Custom Schemas

PUDL schemas are CUE files with embedded `_pudl` metadata that drives inference:

```cue
package ec2

#Instance: {
    _pudl: {
        schema_type:      "base"
        resource_type:    "aws.ec2.instance"
        cascade_priority: 100
        identity_fields:  ["InstanceId"]
        tracked_fields:   ["State", "InstanceType", "Tags"]
    }
    InstanceId:   string
    InstanceType: string
    State:        { Name: string }
    ...
}
```

See [docs/schema-authoring.md](docs/schema-authoring.md) for the full guide.

## Documentation

| Document | Description |
|----------|-------------|
| [docs/concepts.md](docs/concepts.md) | Core concepts: identity, schemas, inference, collections |
| [docs/getting-started.md](docs/getting-started.md) | Installation, first import, first query |
| [docs/cli-reference.md](docs/cli-reference.md) | All commands, flags, and examples |
| [docs/schema-authoring.md](docs/schema-authoring.md) | Writing custom CUE schemas |
| [docs/collections.md](docs/collections.md) | NDJSON, wrapper detection, collection queries |
| [docs/architecture.md](docs/architecture.md) | Streaming, catalog internals, storage layout |
| [docs/model-authoring.md](docs/model-authoring.md) | Writing models |
| [docs/definition-authoring.md](docs/definition-authoring.md) | Writing definitions |
| [docs/method-authoring.md](docs/method-authoring.md) | Writing methods |
| [docs/workflows.md](docs/workflows.md) | Workflow composition |
| [docs/drift.md](docs/drift.md) | Drift detection |
| [docs/vault.md](docs/vault.md) | Vault and credential management |
| [docs/VISION.md](docs/VISION.md) | Project vision and roadmap |
| [docs/TESTING.md](docs/TESTING.md) | Test architecture and coverage |

## Project Status

PUDL has completed all 8 phases of its infrastructure automation expansion. The data pipeline (import, catalog, schemas) and the full automation stack (models, definitions, methods, workflows, drift detection, vault, artifacts, effects, agent integration) are stable and well-tested (352 passing tests across 24 packages).

See [docs/VISION.md](docs/VISION.md) for the full roadmap.

## Requirements

- Go 1.24+
- Git (for schema version control)
- CUE ([cuelang.org](https://cuelang.org)) for schema definitions
