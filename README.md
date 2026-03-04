# PUDL — Personal Unified Data Lake

PUDL is a CLI tool that turns cloud infrastructure data into a queryable, schema-validated personal data lake. Import JSON, YAML, CSV, and NDJSON from AWS, Kubernetes, or any source — PUDL detects formats, infers schemas, tracks provenance, and deduplicates automatically.

Everything runs locally. One binary, SQLite catalog, CUE schemas tracked in a local git repo.

## Quick Start

```bash
# Build and initialize
go build -o pudl .
./pudl init

# Import some data
pudl import --path aws-ec2-instances.json
pudl import --path k8s-pods.yaml
pudl import --path cloud-inventory.ndjson

# Query your catalog
pudl list
pudl list --schema aws/ec2.#Instance --verbose
pudl show mivof-duhij --raw
```

## What Happens When You Import

```
pudl import --path data.json
    │
    ├── SHA256 content hash → deduplicate (skip if already imported)
    ├── Detect format (json, yaml, csv, ndjson)
    ├── Detect if it's a collection wrapper (e.g. {"items": [...], "count": 5})
    ├── Infer schema via heuristics + CUE unification
    ├── Extract resource identity (stable ID across re-imports)
    ├── Store raw file in ~/.pudl/data/raw/YYYY/MM/DD/
    └── Catalog in SQLite with full provenance metadata
```

Data is never rejected — if no specific schema matches, it falls back to the universal `pudl/core.#Item` catchall.

## Key Concepts

- **Content-addressed IDs**: Files are identified by SHA256 hash, displayed as pronounceable [proquint](https://arxiv.org/html/0901.4016) words like `mivof-duhij`
- **CUE schemas with `_pudl` metadata**: Schemas define both data shape and inference hints (identity fields, tracked fields, cascade priority)
- **Schema inference**: Heuristic scoring narrows candidates, then CUE unification validates matches — most specific schema wins
- **Resource identity**: Same logical resource tracked across re-imports via `resource_id` (schema + identity fields hash)
- **Collections**: NDJSON files and JSON API wrappers are automatically split into individual items with parent references

See [docs/concepts.md](docs/concepts.md) for a deeper explanation of these ideas.

## Commands

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
| [docs/VISION.md](docs/VISION.md) | Project vision and roadmap |
| [docs/TESTING.md](docs/TESTING.md) | Test architecture and coverage |

## Project Status

PUDL is in active development. The core import/catalog/schema pipeline is stable and well-tested (291 passing tests). Next priorities are `pudl diff` (compare resource versions), `pudl stats` (aggregate views), and basic outlier detection.

See [docs/VISION.md](docs/VISION.md) for the full roadmap.

## Requirements

- Go 1.24+
- Git (for schema version control)
