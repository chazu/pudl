# Core Concepts

This document explains the key ideas behind PUDL. Understanding these will make everything else — the CLI, the schemas, the catalog — click into place.

## The Import Pipeline

When you run `pudl import --path data.json`, a series of steps happen automatically:

```
File → Content Hash → Format Detection → Collection Detection →
Schema Inference → Identity Extraction → Storage → Catalog
```

Each step is described below.

## Content-Addressed Storage

Every imported file is identified by the **SHA256 hash** of its contents. This serves two purposes:

1. **Deduplication**: If you import the same file twice, the second import is skipped immediately — the content hash already exists in the catalog.
2. **Integrity**: The hash proves the stored data matches what was originally imported.

### Proquint IDs

Raw SHA256 hashes are 64 hex characters — not something you'd want to type or say aloud. PUDL encodes the first 32 bits as a **proquint**: a pronounceable, human-friendly identifier.

```
SHA256: 87e21d152d1240a46293487b54f707069a2e054a39f72f58896b43d67458302c
Proquint: mivof-duhij
```

Proquints follow a consonant-vowel pattern that makes them easy to read, remember, and communicate. You use them everywhere: `pudl show mivof-duhij`, `pudl delete mivof-duhij`.

The full hex hash is stored in the catalog as the canonical `id`. The proquint is a display format derived from it.

## Three Layers of Identity

PUDL tracks three distinct identity concepts:

### 1. Content Hash (file-level dedup)

`content_hash` = SHA256 of the raw file bytes. Two identical files produce the same hash. This is the dedup gate — if the hash already exists, the import is skipped.

### 2. Resource ID (logical identity)

`resource_id` = SHA256 of (normalized schema name + identity field values). This identifies the **logical resource** regardless of which file it came from.

For example, an EC2 instance with `InstanceId: "i-abc123"` always produces the same `resource_id`, whether you import it today or next week. This enables version tracking — you can see how the same resource changed across imports.

For catchall schemas (where no identity fields are defined), the content hash is used as the identity component.

### 3. Entry ID / Proquint (catalog reference)

The `id` stored in the catalog is the full content hash. The proquint is the human-friendly display form. This is what you use in CLI commands.

### How They Relate

```
File bytes ──SHA256──► content_hash (dedup gate)
                           │
Schema + identity fields ──SHA256──► resource_id (logical identity)
                                         │
content_hash ──first 32 bits──► proquint (display ID)
```

When the same logical resource is imported multiple times with different data (e.g., an EC2 instance whose state changed), the `resource_id` stays the same but the `content_hash` and `version` number change.

## CUE Schemas

PUDL uses [CUE](https://cuelang.org/) for schema definition and validation. Schemas live in `~/.pudl/schema/` as `.cue` files, organized into packages (directories).

What makes PUDL schemas special is the `_pudl` metadata block embedded in each definition. This metadata drives the entire inference engine:

```cue
#EC2Instance: {
    _pudl: {
        schema_type:      "base"          // "base", "policy", "custom", "catchall"
        resource_type:    "aws.ec2.instance"
        cascade_priority: 100             // Higher = more specific
        identity_fields:  ["InstanceId"]  // Fields that uniquely identify a resource
        tracked_fields:   ["State", "InstanceType", "Tags"]  // Fields to monitor for changes
    }
    InstanceId:   string
    InstanceType: string
    State:        { Name: string }
    ...
}
```

See [schema-authoring.md](schema-authoring.md) for the full guide on writing schemas.

### Schema Names

Schema names follow a canonical format: `<package-path>.#<Definition>`. Examples:

- `pudl/core.#Item` — the universal catchall
- `pudl/core.#Collection` — collection entries
- `aws/ec2.#Instance` — an AWS EC2 instance

The `#` prefix is a CUE convention for definitions (as opposed to concrete values).

## Schema Inference

When data is imported without a `--schema` flag, PUDL automatically determines the best matching schema. This happens in two phases:

### Phase 1: Heuristic Scoring

Before doing expensive CUE validation, PUDL quickly scores each schema as a candidate based on lightweight checks:

| Signal | Score | Description |
|--------|-------|-------------|
| All identity fields present | +0.5 | Strong match — data has all the key fields the schema expects |
| Partial identity fields | +0.2 × ratio | Weaker signal |
| Tracked fields present | +0.1 × ratio | Supporting evidence |
| Origin matches resource_type | +0.15 | Filename hints match schema's declared resource type |
| Catchall fallback | +0.01 | Always included as last resort |

### Phase 2: CUE Unification

Candidates are tried in order (highest score first, most specific schema first). For each candidate:

1. The data is converted to a CUE value
2. `schema.Unify(data)` merges the schema constraints with the actual data
3. `Validate()` checks if the unified value is consistent

The first schema that validates successfully wins. If nothing matches, the catchall (`pudl/core.#Item`) accepts anything.

### Never-Reject Philosophy

PUDL never rejects data. If your data doesn't match any schema, it's assigned to the catchall with low confidence. You can always write a more specific schema later and run `pudl schema reinfer` to re-classify existing entries.

## Cascade Validation

When you manually specify `--schema` during import, PUDL uses a three-level cascade:

1. **Policy schema** (most restrictive) — e.g., "compliant EC2 instance with required tags"
2. **Base schema** (type recognition) — e.g., "any EC2 instance"
3. **Catchall** — accepts anything

This is driven by the `cascade_priority` and `base_schema` fields in `_pudl` metadata. Higher priority schemas are tried first; if they fail, the cascade falls through to less specific schemas.

## Collections

PUDL handles multi-record data in two ways:

### NDJSON Files

Files containing newline-delimited JSON objects are automatically detected and processed as collections. Each line becomes an individual catalog entry linked to a parent collection entry.

### JSON API Wrappers

JSON objects that look like API responses — `{"items": [...], "count": 5, "next_token": "abc"}` — are automatically detected and unwrapped. The inner array items become individual catalog entries, and the wrapper metadata (pagination, counts) is preserved.

Wrapper detection uses a scoring algorithm that looks for signals like known key names (`items`, `data`, `results`), pagination siblings (`next_token`, `total_count`), count-matches-array-length, and homogeneous elements. A score ≥ 0.50 triggers unwrapping.

### Collection Relationships

```
Collection entry (📦)
    ├── Item 0 (📄) — linked via collection_id, item_index=0
    ├── Item 1 (📄) — linked via collection_id, item_index=1
    └── Item 2 (📄) — linked via collection_id, item_index=2
```

Items inherit provenance from their collection but get their own schema assignment, resource identity, and content hash. You can query items individually or filter by collection.

See [collections.md](collections.md) for the full collection guide.

## Models

Models are a separate concept from schemas. A model *references* one or more schemas and adds methods, sockets, authentication, and metadata. Schemas remain pure data shapes — models layer behavior on top.

### Schema vs Model vs Definition

| Concept | What it is |
|---------|-----------|
| **Schema** | A data shape. CUE constraints describing what a resource looks like. |
| **Model** | A separate entity that references schemas and adds methods, sockets, auth, metadata. |
| **Definition** | A named instance of a model with concrete args. |

Models reference schemas rather than embedding operational behavior into them. This preserves schema purity and enables reuse — the same schema can back multiple models.

### Three Validation Layers

| Layer | What it validates | How |
|-------|------------------|-----|
| **Base schema** | Structural shape — required fields, types | CUE constraints |
| **Policy schemas** | Stricter rules — compliance, security | CUE constraints (cascade) |
| **Qualification methods** | Runtime checks — credential validity, resource existence | Model methods with `kind: "qualification"` |

The key distinction: can CUE validate it statically? If yes, it's a schema constraint. If no (requires API calls, network checks), it's a qualification method on the model. The three layers compose rather than replace each other.

See [model-authoring.md](model-authoring.md) for the full guide on writing models.

## Definitions

Definitions are named instances of models with concrete configuration. While a model describes what a resource type looks like and what operations it supports, a definition assigns specific values and wires instances together.

### Socket Wiring

Definitions connect to each other through socket wiring — one definition's field references another definition's output:

```cue
prod_instance: examples.#EC2InstanceModel & {
    schema: {
        VpcId: prod_vpc.outputs.vpc_id  // wired to VPC output
    }
}
```

CUE validates type compatibility at parse time. The dependency graph is built automatically from these cross-references.

### Dependency Graph

Definitions form a directed acyclic graph (DAG) based on socket wiring. `pudl definition graph` shows the topological ordering — the order in which definitions should be processed so that dependencies are resolved first.

See [definition-authoring.md](definition-authoring.md) for the full guide on writing definitions.

## Workspace Layout

```
~/.pudl/
├── config.yaml                    # Configuration
├── data/
│   ├── raw/YYYY/MM/DD/            # Date-partitioned imported data
│   ├── metadata/                  # Per-import JSON metadata sidecar files
│   └── sqlite/catalog.db          # SQLite catalog database
└── schema/                        # Git-tracked CUE schema repository
    ├── cue.mod/module.cue         # CUE module definition
    └── pudl/
        ├── core/core.cue          # Bootstrap schemas (catchall, collection)
        └── <your packages>/       # AWS, K8s, custom schemas
```

The schema directory is a git repository — `pudl schema status`, `pudl schema commit`, and `pudl schema log` manage its history.
