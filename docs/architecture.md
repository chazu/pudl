# Architecture

This document describes PUDL's internal architecture: storage layout, streaming pipeline, catalog database, and package structure.

## Storage Layout

```
~/.pudl/
├── config.yaml                    # YAML configuration
├── data/
│   ├── raw/YYYY/MM/DD/            # Date-partitioned imported data files
│   │   └── YYYYMMDD_HHMMSS_origin.ext
│   ├── metadata/                  # Per-import JSON metadata sidecar files
│   │   └── YYYYMMDD_HHMMSS_origin.ext.meta
│   ├── sqlite/catalog.db          # SQLite catalog database
│   ├── .runs/                     # Workflow run manifests
│   │   └── <workflow>/<run-id>.json
│   └── .drift/                    # Drift detection reports
│       └── <definition>/<timestamp>.json
├── schema/                        # Git-tracked CUE schema repository
│   ├── .git/                      # Full git repository
│   ├── cue.mod/module.cue         # CUE module definition
│   ├── pudl/
│   │   ├── core/core.cue          # Bootstrap schemas (catchall, collection)
│   │   └── <user packages>/       # Custom schema packages
│   ├── models/                    # Model CUE files
│   ├── definitions/               # Named model instances
│   ├── methods/                   # Glojure method implementations
│   │   └── <model-name>/<method-name>.clj
│   └── extensions/
│       └── models/                # User extension models
└── vaults/                        # Encrypted credential stores
    └── default.age                # age-encrypted secrets file
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
    confidence        REAL,              -- Schema assignment confidence (0.0–1.0)
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

Database migrations run automatically on startup. Adding columns or indexes is done through migration functions that check for existing columns before altering — safe to run on every open.

## Streaming Pipeline

All imports use Content-Defined Chunking (CDC) via `go-cdc-chunkers` for bounded-memory processing.

```
Input File
    │
    ▼
CDC Chunker ── splits file into content-defined chunks
    │
    ▼
Format Processor ── parses JSON/CSV/YAML across chunk boundaries
    │                (maintains state for cross-chunk reassembly)
    ▼
Data Objects ── extracted records
    │
    ▼
Schema Inference ── heuristic scoring + CUE unification
    │
    ▼
Identity Extraction ── resource_id, content_hash, version
    │
    ▼
Catalog + Storage ── SQLite insert + file copy
```

### Content-Defined Chunking

CDC boundaries are determined by the data content itself (not fixed offsets). This makes chunking shift-resilient — inserting data at the beginning doesn't change all subsequent chunk boundaries.

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
    │
    ├── Read file, compute SHA256 → contentHash
    ├── Check catalog: if contentHash exists → skip (dedup)
    │
    ├── detectFormat(path, content) → "json" | "yaml" | "csv" | "ndjson"
    │
    ├── If NDJSON → importNDJSONCollection()
    │       ├── createCollectionEntry()
    │       └── createCollectionItems() → individual entries
    │
    ├── analyzeDataStreaming() → parse via CDC
    │
    ├── If JSON object → DetectCollectionWrapper(data)
    │       If wrapper detected (score ≥ 0.50):
    │           └── importWrappedCollection()
    │                   ├── createCollectionEntry()
    │                   └── createCollectionItems()
    │
    ├── SchemaInferrer.Infer(data, hints)
    │       ├── SelectCandidates() → heuristic scoring
    │       ├── Sort by specificity (most-specific-first)
    │       ├── tryUnify() each candidate → CUE validation
    │       └── Return first match or catchall
    │
    ├── identity.ExtractFieldValues(data, identityFields)
    ├── identity.ComputeResourceID(schema, fieldValues)
    │
    ├── Copy raw file to ~/.pudl/data/raw/YYYY/MM/DD/
    ├── Write metadata sidecar JSON
    └── CatalogDB.AddEntry()
```

## Package Structure

| Package | Path | Responsibility |
|---------|------|----------------|
| `importer` | `internal/importer/` | Import pipeline: format detection, streaming, collections, wrapper detection |
| `importer` (enhanced) | `internal/importer/enhanced_importer.go` | Content-hash dedup wrapper, proquint IDs |
| `inference` | `internal/inference/` | Schema inference: heuristic scoring + CUE unification |
| `identity` | `internal/identity/` | Resource identity: field extraction, ID computation (pure functions) |
| `idgen` | `internal/idgen/` | Content IDs: SHA256, proquint encoding/decoding |
| `database` | `internal/database/` | SQLite catalog: CRUD, migrations, queries |
| `validator` | `internal/validator/` | CUE module loader, cascade validator, validation service |
| `schemaname` | `internal/schemaname/` | Schema name normalization (canonical format) |
| `schemagen` | `internal/schemagen/` | Schema generation from data |
| `streaming` | `internal/streaming/` | CDC chunkers, format-specific processors |
| `typepattern` | `internal/typepattern/` | Type detection for K8s, AWS, GitLab |
| `config` | `internal/config/` | YAML configuration |
| `init` | `internal/init/` | Workspace initialization |
| `git` | `internal/git/` | Git operations on schema repo |
| `lister` | `internal/lister/` | List/query with filters and display options |
| `ui` | `internal/ui/` | Output formatting, interactive TUI |
| `doctor` | `internal/doctor/` | Health checks |
| `errors` | `internal/errors/` | Typed error codes |
| `model` | `internal/model/` | Model discovery, schema reference resolution, method/socket extraction |
| `definition` | `internal/definition/` | Definition loader, validator, socket wiring, dependency graph |
| `glojure` | `internal/glojure/` | Glojure runtime embedding, namespace registry, CUE function bridge |
| `executor` | `internal/executor/` | Lifecycle dispatch, qualification runner, socket value propagation |
| `artifact` | `internal/artifact/` | Artifact serialization, hashing, storage, dedup |
| `vault` | `internal/vault/` | Vault interface, env/file backends, resolution walker |
| `workflow` | `internal/workflow/` | DAG builder, scheduler, runner, manifest writer |
| `drift` | `internal/drift/` | State comparator, report generator |
| `skills` | `internal/skills/` | Agent skill file management, embedding |
| `effects` | `internal/effects/` | Effect type parsing, formatting |
| `cmd` | `cmd/` | CLI command definitions (Cobra) |

## Two Execution Layers

PUDL has two distinct execution layers that share the same Glojure runtime but serve different purposes.

### CUE Functions (Op Layer)

CUE functions run **during CUE evaluation** to compute values that become part of the CUE tree. They are invoked via `op.#Function & { args: [...] }` syntax inside CUE files. The CUE processor intercepts these during AST walking, calls into the function registry (Go or Glojure implementations), and splices the result back into the CUE value.

CUE functions may perform I/O (HTTP requests, file reads) to fetch values, but they are not part of the lifecycle dispatch system. They answer "what is this field's value?" — not "should we proceed?" or "go create this resource."

### Methods (Execution Layer)

Methods run **during `pudl method run`** against resolved definitions. They participate in lifecycle dispatch: advice (qualifications) gates actions, attributes derive values post-action, codegen produces output transforms. Method results are stored as immutable artifacts in the catalog.

### Comparison

| | CUE functions (op layer) | Methods (execution layer) |
|---|---|---|
| **Purpose** | Compute/fetch a **value** for the CUE tree | Perform an **operation** on infrastructure |
| **When** | During CUE evaluation | During `pudl method run` |
| **Returns** | A CUE-compatible value | Artifacts, status, socket outputs |
| **May do I/O?** | Yes (HTTP, file reads, etc.) | Yes |
| **Lifecycle dispatch?** | No — called and returns | Yes — advice gates, ordering, propagation |
| **Idempotent expectation?** | Yes — same inputs, same value | Not necessarily (create, delete) |
| **Stored?** | No — value lives in the CUE tree | Yes — artifacts in catalog |

### Design Rationale

The boundary is **purpose**, not **purity**. Both layers can perform I/O, but they answer different questions:

- **"What value should this field have?"** → CUE function. Example: fetch the latest AMI ID from AWS and use it in a definition field.
- **"Is this safe to proceed?"** → Advice method. Example: check whether AWS credentials are valid before launching an instance.
- **"Do this thing."** → Action method. Example: launch the EC2 instance.
- **"What can we derive from the result?"** → Attribute method. Example: compute a cost estimate from instance type + region.

Both layers share the same embedded Glojure runtime and function registry. CUE functions are registered in the `op` package; methods live in `.clj` files under `methods/`. This avoids two separate runtimes while keeping the invocation paths cleanly separated.

Because CUE functions can do I/O, the CUE processor must handle timeouts, caching (same function+args across definitions should not fetch twice), and error reporting that distinguishes eval-time failures from execution-time failures.

## Technology Stack

- **Go 1.24** — core application
- **Cobra** — CLI framework
- **CUE** (`cuelang.org/go v0.14`) — schema definition, validation, unification
- **SQLite** (`go-sqlite3`) — catalog database
- **go-cdc-chunkers** — Content-Defined Chunking for streaming
- **Bubbletea + Bubbles + Lipgloss** — interactive TUI (`pudl list --fancy`)
- **yaml.v3** — YAML config and data parsing
- **Glojure** (`github.com/glojurelang/glojure v0.6.4`) — Clojure-like scripting for method implementations
- **age** — encryption for file-based vault
- **testify** — test assertions
