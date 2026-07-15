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

**NDJSON files** are automatically detected and processed as collections. Each line
becomes an individual catalog entry linked to a parent collection entry. Observe
ingestion uses the same membership mechanism for a timestamped snapshot collection.

**Typed envelopes** use the shape `{"schema": {"module": ..., "version": ...},
"definitions": [...], "data": ...}`. The envelope metadata is recorded in
`item_schemas`; inline CUE definitions are cached when present, and only the inner
`data` payload is passed to normal import and inference. Ordinary JSON objects are
not guessed to be collection wrappers.

Collection items get their own schema assignment, resource identity, and content hash.
Memberships are stored separately, so a content-addressed item can belong to more than
one collection without duplicating its catalog row.

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

## System Models

A model is a `#SystemModel` instance -- a named CUE value that declares the desired shape of a slice of your system. Where a schema describes the shape of a resource type, a model declares concrete intent: how to observe the world (`populate`), what the world should look like (`desired`), and what invariants must hold (`checks`).

A model's `desired` block is a set of named resources. Each resource is the per-status
unit: it declares the intended state for one thing the model is responsible for.

Use `pudl model list` to see available models, `pudl model show <name>` to inspect one, and `pudl model validate <name>` to check that a model parses and conforms to `#SystemModel`.

## Running a Model

`pudl run <name>` drives a model through an observe-only ACUTE loop:

1. **Populate** -- imports the model's declared sources into the catalog, refreshing observations.
2. **Drift** -- compares the model's `desired` state against observed state. The mode is auto-detected from the observer: a *differential* observer (k8s) reads `desired` as sources and reports per-resource exists/matches, while an *inventory* observer (a `host`-style plugin or an `#EweTarget` fetcher) dumps records that pudl set-diffs against `desired`. A `#PluginObserve` arm defaults to differential; set `differential: false` for inventory observers. `--from-catalog` forces inventory drift from already-ingested records.
3. **Checks** -- evaluates the model's invariants against current facts.
4. **Report** -- records a verdict for the run.

By default `pudl run` observes only: it computes drift but changes nothing in the world.

### Convergence

`pudl run <name> --converge` closes drift instead of merely reporting it. pudl renders the model's `desired` state to sources, and the mu plugin reconciles those sources against the real system. pudl declares state; mu executes. There is no separate export step.

`pudl status` reads convergence status from the catalog -- each model run records its verdict (`unknown | drifted | converging | clean | failed`), so `pudl status` reflects whether the system is in sync (`clean`) or drifted. `clean` is the single in-sync state, written only when a drift re-check observes ∅ and the apply receipt was persisted; an out-of-band apply (`mu build --emit-manifest | pudl mu ingest-manifest --model <name>`) records `converging` until that re-check confirms it. `unknown` also covers an apply whose receipt could not be persisted and therefore needs verification.

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
