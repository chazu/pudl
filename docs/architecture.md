# Architecture

This document describes PUDL's internal architecture: storage layout, streaming pipeline, catalog database, and package structure.

## Storage Layout

```
~/.pudl/
+-- config.yaml                    # YAML configuration
+-- data/
|   +-- raw/YYYY/MM/DD/            # Date-partitioned imported data files
|   |   +-- YYYYMMDD_HHMMSS_origin.ext
|   +-- metadata/                  # Per-import JSON metadata sidecar files
|   |   +-- YYYYMMDD_HHMMSS_origin.ext.meta
|   +-- sqlite/catalog.db          # SQLite catalog database
|   +-- .drift/                    # Drift detection reports
|       +-- <definition>/<timestamp>.json
+-- schema/                        # Git-tracked CUE schema repository
|   +-- .git/                      # Full git repository
|   +-- cue.mod/module.cue         # CUE module definition
|   +-- pudl/
|   |   +-- core/core.cue          # Bootstrap schemas (catchall, collection)
|   |   +-- <user packages>/       # Custom schema packages
|   +-- definitions/               # Named schema instances
|   +-- extensions/
|       +-- models/                # User extension models
+-- vaults/                        # Encrypted credential stores
    +-- default.age                # age-encrypted secrets file
```

### Raw Data

Imported files are copied to `data/raw/` with date-based partitioning (`YYYY/MM/DD/`). The original file content is preserved exactly as imported.

### Metadata Sidecars

Each import produces a JSON metadata file alongside the raw data. This includes import timestamp, detected format, origin, schema assignment, confidence score, and collection relationships.

### Schema Repository

The schema directory is a standalone git repository. Bootstrap schemas (`pudl/core.#Item` and `pudl/core.#Collection`) are embedded in the PUDL binary and copied to the user's schema repo on `pudl init`. User schemas are added alongside them.

## Catalog Database

The catalog uses SQLite with WAL mode for concurrent read safety.

### Schema

```sql
catalog_entries (
    id                TEXT PRIMARY KEY,  -- SHA256 content hash (hex)
    stored_path       TEXT,              -- Path to raw data file
    metadata_path     TEXT,              -- Path to metadata sidecar
    import_timestamp  DATETIME,
    format            TEXT,              -- json, yaml, csv, ndjson
    origin            TEXT,              -- Data source identifier
    schema            TEXT,              -- Assigned CUE schema name
    confidence        REAL,              -- Schema assignment confidence (0.0-1.0)
    record_count      INTEGER,
    size_bytes        INTEGER,
    content_hash      TEXT,              -- SHA256 of file content
    resource_id       TEXT,              -- Logical resource identity hash
    version           INTEGER,           -- Version number for same resource_id
    collection_id     TEXT,              -- Parent collection ID (NULL for standalone)
    item_index        INTEGER,           -- Position in collection (NULL for collections)
    collection_type   TEXT,              -- 'collection', 'item', or NULL
    item_id           TEXT,              -- Unique item identifier within collection
    entry_type        TEXT,              -- 'import' or 'artifact'
    definition        TEXT,              -- Definition name (artifacts only)
    method            TEXT,              -- Method name (artifacts only)
    run_id            TEXT,              -- Execution run ID (artifacts only)
    tags              TEXT,              -- JSON-encoded key-value tags
    created_at        DATETIME,
    updated_at        DATETIME
)
```

### Indexes

Optimized indexes on `schema`, `origin`, `format`, `collection_id`, `collection_type`, and `import_timestamp` for fast filtered queries.

### Configuration

- WAL mode for concurrent reads
- 10,000 page cache
- ACID compliance via transactions
- Idempotent migrations (safe on every DB open)

### Migrations

Database migrations run automatically on startup. Adding columns or indexes is done through migration functions that check for existing columns before altering -- safe to run on every open.

## Streaming Pipeline

All imports use Content-Defined Chunking (CDC) via `go-cdc-chunkers` for bounded-memory processing.

```
Input File
    |
    v
CDC Chunker -- splits file into content-defined chunks
    |
    v
Format Processor -- parses JSON/CSV/YAML across chunk boundaries
    |                (maintains state for cross-chunk reassembly)
    v
Data Objects -- extracted records
    |
    v
Schema Inference -- heuristic scoring + CUE unification
    |
    v
Identity Extraction -- resource_id, content_hash, version
    |
    v
Catalog + Storage -- SQLite insert + file copy
```

### Content-Defined Chunking

CDC boundaries are determined by the data content itself (not fixed offsets). This makes chunking shift-resilient -- inserting data at the beginning does not change all subsequent chunk boundaries.

### Format Processors

Each format has a chunk processor that handles:
- **JSON**: Object/array boundary detection, cross-chunk reassembly
- **CSV**: Row boundary detection, header tracking
- **YAML**: Document boundary detection (`---` separators)
- **NDJSON**: Line-by-line parsing with individual item extraction

Processors implement `ProcessChunk()`, `Finalize()`, `Reset()`, and `GetBufferSize()`.

### Memory Management

Configurable via CLI flags:
- `--streaming-memory`: Total memory limit in MB (default: 100)
- `--streaming-chunk-size`: Average chunk size in MB (default: 0.016)

Small files (< 10KB) use smaller chunks for efficient processing. Large files use the configured chunk size.

## Import Flow

The complete import path through `EnhancedImporter`:

```
ImportFileWithFriendlyIDs(opts)
    |
    +-- Read file, compute SHA256 -> contentHash
    +-- Check catalog: if contentHash exists -> skip (dedup)
    |
    +-- detectFormat(path, content) -> "json" | "yaml" | "csv" | "ndjson"
    |
    +-- If NDJSON -> importNDJSONCollection()
    |       +-- createCollectionEntry()
    |       +-- createCollectionItems() -> individual entries
    |
    +-- analyzeDataStreaming() -> parse via CDC
    |
    +-- If JSON object -> DetectCollectionWrapper(data)
    |       If wrapper detected (score >= 0.50):
    |           +-- importWrappedCollection()
    |                   +-- createCollectionEntry()
    |                   +-- createCollectionItems()
    |
    +-- SchemaInferrer.Infer(data, hints)
    |       +-- SelectCandidates() -> heuristic scoring
    |       +-- Sort by specificity (most-specific-first)
    |       +-- tryUnify() each candidate -> CUE validation
    |       +-- Return first match or catchall
    |
    +-- identity.ExtractFieldValues(data, identityFields)
    +-- identity.ComputeResourceID(schema, fieldValues)
    |
    +-- Copy raw file to ~/.pudl/data/raw/YYYY/MM/DD/
    +-- Write metadata sidecar JSON
    +-- CatalogDB.AddEntry()
```

## Package Structure

| Package | Path | Responsibility |
|---------|------|----------------|
| `config` | `internal/config/` | YAML configuration loading and defaults |
| `database` | `internal/database/` | SQLite catalog: CRUD, migrations, queries |
| `definition` | `internal/definition/` | Definition loader, validator, socket wiring, dependency graph |
| `doctor` | `internal/doctor/` | Workspace health checks |
| `drift` | `internal/drift/` | State comparator, report generator and storage |
| `errors` | `internal/errors/` | Typed error codes for CLI error handling |
| `git` | `internal/git/` | Git operations on schema repository |
| `identity` | `internal/identity/` | Resource identity: field extraction, ID computation (pure functions) |
| `idgen` | `internal/idgen/` | Content IDs: SHA256, proquint encoding/decoding |
| `importer` | `internal/importer/` | Import pipeline: format detection, streaming, collections, wrapper detection |
| `importer` (enhanced) | `internal/importer/enhanced_importer.go` | Content-hash dedup wrapper, proquint IDs |
| `inference` | `internal/inference/` | Schema inference: heuristic scoring + CUE unification |
| `init` | `internal/init/` | Workspace initialization and auto-init |
| `lister` | `internal/lister/` | List/query with filters and display options |
| `mubridge` | `internal/mubridge/` | Drift-to-mu action spec export bridge |
| `repo` | `internal/repo/` | Repository-level init and validation |
| `schema` | `internal/schema/` | Schema file management and discovery |
| `schemagen` | `internal/schemagen/` | Schema generation from imported data |
| `schemaname` | `internal/schemaname/` | Schema name normalization (canonical format) |
| `skills` | `internal/skills/` | Agent skill file management and embedding |
| `streaming` | `internal/streaming/` | CDC chunkers, format-specific chunk processors |
| `typepattern` | `internal/typepattern/` | Type detection for K8s, AWS, GitLab registries |
| `ui` | `internal/ui/` | Output formatting, interactive TUI (bubbletea) |
| `validator` | `internal/validator/` | CUE module loader, cascade validator, validation service |
| `vault` | `internal/vault/` | Vault interface, env/file backends, resolution walker |
| `cmd` | `cmd/` | CLI command definitions (Cobra) |

## Core Data Flow

PUDL is a data import, cataloging, and validation system. The core flow is:

```
                    +------------------+
                    |   File Input     |
                    | (JSON/YAML/CSV/  |
                    |  NDJSON)         |
                    +--------+---------+
                             |
                             v
                    +------------------+
                    | Format Detection |
                    | + CDC Streaming  |
                    +--------+---------+
                             |
                             v
                    +------------------+
                    | Schema Inference |
                    | (heuristics +    |
                    |  CUE unify)      |
                    +--------+---------+
                             |
                             v
                    +------------------+
                    | Identity Extract |
                    | (resource_id,    |
                    |  content_hash,   |
                    |  version)        |
                    +--------+---------+
                             |
                             v
              +--------------+--------------+
              |                             |
              v                             v
    +------------------+          +------------------+
    | Raw File Storage |          | SQLite Catalog   |
    | (date-partitioned|          | (metadata, schema|
    |  + metadata)     |          |  assignment, ID) |
    +------------------+          +------------------+
```

After import, data can be:
- **Queried** via `pudl list` with filters on schema, origin, format, collection membership
- **Validated** against assigned schemas via `pudl validate`
- **Exported** in various formats via `pudl export`
- **Monitored for drift** via `pudl drift check` against definition state
- **Re-classified** when schemas change via `pudl schema reinfer`

## Technology Stack

- **Go 1.24** -- core application
- **Cobra** -- CLI framework
- **CUE** (`cuelang.org/go v0.14`) -- schema definition, validation, unification
- **SQLite** (`go-sqlite3`) -- catalog database
- **go-cdc-chunkers** -- Content-Defined Chunking for streaming
- **Bubbletea + Bubbles + Lipgloss** -- interactive TUI (`pudl list --fancy`)
- **yaml.v3** -- YAML config and data parsing
- **age** -- encryption for file-based vault
- **testify** -- test assertions
