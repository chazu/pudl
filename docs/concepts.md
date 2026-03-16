# Core Concepts

This document explains the key ideas behind PUDL. Understanding these will make everything else -- the CLI, the schemas, the catalog -- click into place.

## Data Import Pipeline

When you run `pudl import --path data.json`, a series of steps happen automatically:

```
File -> Content Hash -> Format Detection -> Collection Detection ->
Schema Inference -> Identity Extraction -> Storage -> Catalog
```

### Format Detection

PUDL auto-detects file format from content and extension. Supported formats:

- **JSON** -- single objects or arrays
- **NDJSON** -- newline-delimited JSON, automatically split into collections
- **YAML** / **YML**
- **CSV**

### Collection Handling

PUDL handles multi-record data in two ways:

**NDJSON files** are automatically detected and processed as collections. Each line becomes an individual catalog entry linked to a parent collection entry.

**JSON API wrappers** -- objects that look like API responses (`{"items": [...], "count": 5, "next_token": "abc"}`) -- are automatically detected and unwrapped. Wrapper detection uses a scoring algorithm that looks for known key names (`items`, `data`, `results`), pagination siblings (`next_token`, `total_count`), count-matches-array-length, and homogeneous elements. A score of 0.50 or higher triggers unwrapping.

Collection items inherit provenance from their parent but get their own schema assignment, resource identity, and content hash. You can query items individually or filter by collection.

### Streaming

All imports use streaming by default. For very large files, you can tune memory usage with `--streaming-memory` and `--streaming-chunk-size`.

## Content-Addressed Identity

Every imported file is identified by the **SHA256 hash** of its contents. This serves two purposes:

1. **Deduplication**: If you import the same file twice, the second import is skipped immediately -- the content hash already exists in the catalog.
2. **Integrity**: The hash proves the stored data matches what was originally imported.

### Proquint IDs

Raw SHA256 hashes are 64 hex characters -- not something you would want to type or say aloud. PUDL encodes the first 32 bits as a **proquint**: a pronounceable, human-friendly identifier.

```
SHA256: 87e21d152d1240a46293487b54f707069a2e054a39f72f58896b43d67458302c
Proquint: mivof-duhij
```

Proquints follow a consonant-vowel pattern that makes them easy to read, remember, and communicate. You use them everywhere: `pudl show mivof-duhij`, `pudl delete mivof-duhij`.

The full hex hash is stored in the catalog as the canonical `id`. The proquint is a display format derived from it.

### Three Layers of Identity

PUDL tracks three distinct identity concepts:

**Content Hash (file-level dedup)** -- `content_hash` is the SHA256 of the raw file bytes. Two identical files produce the same hash. This is the dedup gate -- if the hash already exists, the import is skipped.

**Resource ID (logical identity)** -- `resource_id` is the SHA256 of the normalized schema name combined with identity field values. This identifies the logical resource regardless of which file it came from. For example, an EC2 instance with `InstanceId: "i-abc123"` always produces the same `resource_id`, whether you import it today or next week. This enables version tracking. For catchall schemas (where no identity fields are defined), the content hash is used as the identity component.

**Entry ID / Proquint (catalog reference)** -- The `id` stored in the catalog is the full content hash. The proquint is the human-friendly display form used in CLI commands.

```
File bytes ---SHA256---> content_hash (dedup gate)

Schema + identity fields ---SHA256---> resource_id (logical identity)

content_hash ---first 32 bits---> proquint (display ID)
```

When the same logical resource is imported multiple times with different data (e.g., an EC2 instance whose state changed), the `resource_id` stays the same but the `content_hash` and `version` number change.

## Schema System

PUDL uses [CUE](https://cuelang.org/) for schema definition and validation. Schemas live in `~/.pudl/schema/` as `.cue` files, organized into packages (directories).

What makes PUDL schemas special is the `_pudl` metadata block embedded in each definition. This metadata drives the inference engine:

```cue
#EC2Instance: {
    _pudl: {
        schema_type:     "base"          // "base", "custom", "catchall"
        resource_type:   "aws.ec2.instance"
        identity_fields: ["InstanceId"]  // Fields that uniquely identify a resource
        tracked_fields:  ["State", "InstanceType", "Tags"]  // Fields to monitor for changes
    }
    InstanceId:   string
    InstanceType: string
    State:        { Name: string }
    ...
}
```

The `_pudl` metadata fields are:

- **schema_type** -- Classifies the schema as `"base"`, `"custom"`, or `"catchall"`.
- **resource_type** -- A dotted identifier for the kind of resource (e.g., `"aws.ec2.instance"`).
- **identity_fields** -- Fields that uniquely identify a resource instance. Used to compute the `resource_id`.
- **tracked_fields** -- Fields monitored for changes across imports. Used for drift detection.

### Schema Names

Schema names follow a canonical format: `<package>.#<Definition>`. Examples:

- `pudl/core.#Item` -- the universal catchall
- `pudl/core.#Collection` -- collection entries
- `aws/ec2.#Instance` -- an AWS EC2 instance

The `#` prefix is a CUE convention for definitions (as opposed to concrete values).

## Schema Inference

When data is imported without a `--schema` flag, PUDL automatically determines the best matching schema. This happens in two phases:

### Phase 1: Heuristic Scoring

Before doing expensive CUE validation, PUDL quickly scores each schema as a candidate based on lightweight checks:

| Signal | Score | Description |
|--------|-------|-------------|
| All identity fields present | +0.5 | Strong match -- data has all the key fields the schema expects |
| Partial identity fields | +0.2 x ratio | Weaker signal |
| Tracked fields present | +0.1 x ratio | Supporting evidence |
| Origin matches resource_type | +0.15 | Filename hints match schema's declared resource type |
| Catchall fallback | +0.01 | Always included as last resort |

### Phase 2: CUE Unification

Candidates are tried in order (highest score first, most specific schema first). For each candidate:

1. The data is converted to a CUE value
2. `schema.Unify(data)` merges the schema constraints with the actual data
3. `Validate()` checks if the unified value is consistent

The first schema that validates successfully wins. If nothing matches, the catchall (`pudl/core.#Item`) accepts anything.

## Validation

PUDL validates data against schemas using native CUE unification. When a schema is specified explicitly via `--schema`, validation follows a three-step fallback:

1. **Intended schema** -- the schema you specified
2. **Base schema** -- a less restrictive schema for the same resource type
3. **Catchall** -- `pudl/core.#Item`, which accepts anything

This ensures data is always accepted at some level. If the intended schema rejects the data, it falls through to progressively less specific schemas.

### Never-Reject Philosophy

PUDL never rejects data. If your data does not match any schema, it is assigned to the catchall with low confidence. You can always write a more specific schema later and run `pudl schema reinfer` to re-classify existing entries.

## Definitions

Definitions are named CUE values that conform to schemas and carry concrete configuration. While a schema describes the shape of a resource type, a definition assigns specific values.

### Socket Wiring

Definitions connect to each other through socket wiring -- one definition's field references another definition's output:

```cue
prod_instance: examples.#EC2Instance & {
    VpcId: prod_vpc.outputs.vpc_id  // wired to VPC definition
}
```

CUE validates type compatibility at parse time. The dependency graph is built automatically from these cross-references.

### Dependency Graph

Definitions form a directed acyclic graph (DAG) based on socket wiring. `pudl definition graph` shows the topological ordering -- the order in which definitions should be processed so that dependencies are resolved first.

## Drift Detection

Drift detection compares a definition's declared state (its socket bindings and field values) against the actual state from imported data. The comparison produces a JSON deep diff reporting added, removed, and changed fields with dot-notation paths.

Use `pudl drift check <definition>` to check one definition, or `--all` to check everything.

Drift reports are stored in `.pudl/data/.drift/<definition>/<timestamp>.json`.

### mu Bridge

Drift reports can be exported as mu-compatible action specs using `pudl export-actions`. Each field difference in a drift report becomes an ActionSpec, bridging pudl's drift knowledge to mu's execution engine.

## Fixed-Point Verification

The `pudl verify` command re-runs schema inference on all catalog entries and confirms every entry still resolves to the same schema it was originally assigned. This is an idempotency check: if inference is deterministic, re-running it on stored data should always produce the same schema assignment. Any mismatch indicates drift between the stored schema and the current inference rules.

## Catalog Layer

The catalog (`pudl catalog`) is a central registry of all known schema types. It lists each registered type along with its `schema_type`, `resource_type`, and description. This includes built-in types (`pudl/core.#Item`, `pudl/core.#Collection`) and any user-defined types that include `_pudl` metadata.

## Doctor

The `pudl doctor` command runs health checks on your workspace, including:

- Workspace structure (required directories)
- Database integrity
- Schema repository setup
- Git repository initialization
- Directory structure validation
- Orphaned files

## Workspace Layout

```
~/.pudl/
├── config.yaml                    # Configuration
├── data/
│   ├── raw/YYYY/MM/DD/            # Date-partitioned imported data
│   ├── metadata/                  # Per-import JSON metadata sidecar files
│   ├── sqlite/catalog.db          # SQLite catalog database
│   └── .drift/                    # Drift detection reports
├── schema/                        # Git-tracked CUE schema repository
│   ├── cue.mod/module.cue         # CUE module definition
│   ├── pudl/
│   │   ├── core/core.cue          # Bootstrap schemas (catchall, collection)
│   │   └── <your packages>/       # AWS, K8s, custom schemas
│   └── definitions/               # Named definition instances
```

The schema directory is a git repository -- `pudl schema status`, `pudl schema commit`, and `pudl schema log` manage its history.
