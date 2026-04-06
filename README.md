# PUDL -- Personal Unified Data Lake

PUDL is a CLI tool for building a local, schema-validated data lake. Import JSON, YAML, CSV, and NDJSON from any source -- PUDL detects formats, infers CUE schemas, deduplicates by content hash, tracks provenance, and detects drift. Everything runs locally: one binary, a SQLite catalog, and CUE schemas in a git-tracked directory.

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

# Check schema health
pudl verify
pudl drift check my-server
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

Data is never rejected -- if no specific schema matches, it falls back to the universal `pudl/core.#Item` catchall.

## Key Concepts

- **Content-addressed IDs**: Files are identified by SHA256 hash, displayed as pronounceable [proquint](https://arxiv.org/html/0901.4016) words like `mivof-duhij`
- **CUE schemas with `_pudl` metadata**: Schemas define both data shape and inference hints (identity fields, tracked fields)
- **Schema inference**: Heuristic scoring narrows candidates, then CUE unification validates matches -- most specific schema wins
- **Resource identity**: Same logical resource tracked across re-imports via `resource_id` (schema + identity fields hash)
- **Collections**: NDJSON files and JSON API wrappers are automatically split into individual items with parent references
- **Definitions**: Named configurations validated against CUE schemas, with socket-based wiring between them
- **Drift detection**: Compare declared definition state against imported data using deep diff
- **Bitemporal fact store**: General-purpose store for typed assertions (observations, dependencies, derived facts) with full valid-time and transaction-time tracking
- **mu bridge**: Export drift reports as action specs for the [mu](https://github.com/...) build tool

See [docs/concepts.md](docs/concepts.md) for a deeper explanation of these ideas.

## Commands

### Data Import and Catalog

| Command | Description |
|---------|-------------|
| `pudl init` | Initialize workspace (`~/.pudl/`) |
| `pudl import --path <file>` | Import data with automatic detection |
| `pudl list` | Query catalog (filter by `--schema`, `--origin`, `--format`, etc.) |
| `pudl show <id>` | Inspect an entry (`--raw`, `--metadata`, `--validation`) |
| `pudl delete <id>` | Remove entry from catalog |
| `pudl export` | Export data by ID, schema, or origin to JSON/YAML/CSV/NDJSON |
| `pudl catalog` | List all registered schema types with metadata |

### Schema Management

| Command | Description |
|---------|-------------|
| `pudl schema list` | List schemas (`--package`, `--verbose`) |
| `pudl schema add <name> <file>` | Add a schema to the repository |
| `pudl schema new --from <id>` | Generate CUE schema from imported data |
| `pudl schema show <name>` | Display schema details |
| `pudl schema validate` | Validate schemas |
| `pudl schema migrate` | Run schema migrations |
| `pudl schema reinfer` | Re-infer schemas for existing entries |

### Definitions and Drift

| Command | Description |
|---------|-------------|
| `pudl definition list` | List definitions |
| `pudl definition show <name>` | Show definition details |
| `pudl definition validate` | Validate definitions against their schemas |
| `pudl definition graph` | Show dependency graph |
| `pudl drift check` | Compare declared vs live state |
| `pudl drift report` | Show last drift report |
| `pudl export-actions` | Bridge drift reports to mu action specs |

### Observations and Facts

| Command | Description |
|---------|-------------|
| `pudl observe <description>` | Record a structured observation (`--kind`, `--scope`, `--source`) |
| `pudl facts list` | Query facts by relation with temporal filtering (`--as-of-valid`, `--as-of-tx`) |

See [docs/facts.md](docs/facts.md) for the bitemporal fact store documentation.

### Workspace Operations

| Command | Description |
|---------|-------------|
| `pudl verify` | Fixed-point check: re-run inference on all entries, confirm stability |
| `pudl doctor` | Workspace health checks |
| `pudl repo init` | Initialize `.pudl/` in a repository, install Claude skills |
| `pudl repo validate` | Validate all schemas and definitions |
| `pudl config` | Show current configuration |
| `pudl validate` | Validate data against schemas |

See [docs/cli-reference.md](docs/cli-reference.md) for the full command reference.

## Writing Custom Schemas

PUDL schemas are CUE files with embedded `_pudl` metadata that drives inference:

```cue
package ec2

#Instance: {
    _pudl: {
        schema_type:     "base"
        resource_type:   "aws.ec2.instance"
        identity_fields: ["InstanceId"]
        tracked_fields:  ["State", "InstanceType", "Tags"]
    }
    InstanceId:   string
    InstanceType: string
    State:        { Name: string }
    ...
}
```

See [docs/schema-authoring.md](docs/schema-authoring.md) for the full guide.

## The mu Integration

PUDL knows what has drifted but does not execute changes itself. The `pudl export-actions` command emits JSON action specs that the mu build tool can consume and execute. This separation keeps PUDL focused on data and schema correctness while mu handles orchestration.

```bash
# Check what drifted
pudl drift check my-server

# Export as mu-compatible action plan
pudl export-actions --definition my-server
```

## Documentation

See [docs/README.md](docs/README.md) for the full documentation index.

## Project Status

PUDL's data pipeline (import, catalog, schema inference, drift detection, mu bridge) is stable. The execution layer (models, methods, workflows, Glojure runtime) was removed in a major refactoring to focus PUDL on what it does best: ingesting data, inferring schemas, and detecting drift. Execution is now delegated to mu.

See [docs/VISION.md](docs/VISION.md) for the roadmap.

## Requirements

- Go 1.24+
- Git (for schema version control)
- CUE ([cuelang.org](https://cuelang.org)) for schema definitions
