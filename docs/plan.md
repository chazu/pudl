# PUDL - Development Plan

## Project Overview

PUDL (Personal Unified Data Lake) is a CLI tool for SRE/platform engineers to manage and analyze cloud infrastructure data. It creates a personal data lake for cloud resources, Kubernetes objects, logs, and metrics using CUE-based schema validation.

## Current State

### Core Functionality (Implemented)
- **Data Import** - JSON, YAML, CSV, NDJSON with automatic format detection
- **Collection Support** - Collections split into individual items with parent references
- **SQLite Catalog** - Query, filter, pagination, and provenance tracking
- **CUE Schema Management** - Loading, validation, git version control, schema lifecycle
- **CUE-based Schema Inference** - Automatic detection using heuristics + CUE unification
- **Cascade Validation** - Multi-level schema matching (specific -> fallback -> catchall)
- **Schema Generation** - `pudl schema new` generates CUE schemas from imported data
- **Schema Name Normalization** - Canonical format for consistent schema references
- **Bootstrap Schemas** - Base schemas (catchall, collections) embedded and copied on init
- **Export** - Multi-format output support
- **Delete** - Remove catalog entries
- **Doctor** - Health check utility

### CLI Commands
- `pudl init` - Initialize the data lake
- `pudl import` - Import data files
- `pudl list` - Query and filter catalog entries
- `pudl export` - Export data in various formats
- `pudl delete` - Remove catalog entries
- `pudl validate` - Validate data against schemas
- `pudl schema list` - List available schemas by package
- `pudl schema add` - Add a new schema file
- `pudl schema new` - Generate schema from imported data
- `pudl schema show` - Display schema contents
- `pudl schema edit` - Open schema in editor
- `pudl schema reinfer` - Re-run schema inference on existing entries
- `pudl schema migrate` - Migrate schema names to canonical format
- `pudl schema status` - Show uncommitted schema changes
- `pudl schema commit` - Commit schema changes
- `pudl schema log` - Show schema commit history
- `pudl doctor` - Health check utility
- `pudl migrate identity` - Backfill resource identity tracking for existing entries

### Schema Infrastructure
- `internal/importer/bootstrap/` - Embedded bootstrap CUE schemas
- `internal/schema/manager.go` - Schema loading and management
- `internal/validator/` - CUE validation, cascade validation, and validation service
- `internal/inference/` - CUE-based schema inference with heuristics
- `internal/schemaname/` - Schema name normalization
- `internal/schemagen/` - Schema generation from data

### User Repository (`~/.pudl/`)
```
~/.pudl/
├── config.yaml           # Configuration file
├── data/                  # Imported data files
├── schema/
│   ├── cue.mod/
│   │   └── module.cue    # CUE module
│   ├── pudl/
│   │   ├── core/         # Core schemas (catchall)
│   │   ├── collections/  # Collection schemas
│   │   └── ...           # User-created schema packages
│   └── ...
└── catalog.db            # SQLite catalog
```

## Completed Work

### 2026-01-29 Cleanup
- [x] Removed `internal/importer/cue.mod/` - Was incorrectly creating CUE module in project repo
- [x] Removed `internal/importer/pudl/` - Duplicate of bootstrap/pudl directory
- [x] Consolidated CUE module creation
- [x] Simplified `detectOrigin()` - Uses filename only; schema matching handled by CUE
- [x] Updated tests to reflect simplified detection

### 2026-02-06 Codebase Cleanup
- [x] Removed `internal/review/` - Interactive review workflow removed (untested, unused)
- [x] Moved `ValidationService` from `internal/review/` to `internal/validator/`
- [x] Removed `cmd/git.go` - Redundant `pudl git cd` command
- [x] Split `cmd/schema.go` (~1900 lines) into focused files (~9 files, each under 300 lines)
- [x] Fixed root command description (removed stale Lisp reference)
- [x] Updated `docs/VISION.md` to separate existing features from aspirational

### 2026-02-06 Resource Identity Tracking
- [x] Created `internal/identity/` package — field extraction + resource ID computation
- [x] Database schema evolution — new columns, migrations, identity query methods
- [x] Import flow integration — content hash dedup, identity extraction, versioning
- [x] Reinfer integration — recompute identity when schema changes
- [x] Lister and CLI updates — version display, identity fields in verbose mode
- [x] Backfill command — `pudl migrate identity` for pre-existing entries
- [x] Unit + integration tests (30 new tests)

### 2026-02-09 Streaming Parser Fix & Schema Generation
- [x] Fixed CUE field name quoting — Fields with special characters now properly quoted
- [x] Added pre-write schema validation — `ValidateCUEContent()` validates before writing
- [x] Fixed cascade fallback path format — Uses `"pudl/core.#Item"` not `"core.#Item"`
- [x] Fixed CDC EOF handling — Final chunk now processed when `io.EOF` returned with data
- [x] Fixed NDJSON false positive detection — Only count lines at column 0
- [x] Implemented cross-chunk reassembly — Processor state persisted across chunks
- [x] Added `Finalize()`, `Reset()`, `GetBufferSize()` to ChunkProcessor interface
- [x] Large file tests — 6 new tests for cross-chunk reassembly up to 1MB

### 2026-02-18 Collection Wrapper Detection Research
- [x] pudl-yqt: Researched built-in schema for API collection wrapper responses
- [x] pudl-mxk: Deep dive into Option B (CUE schema-based detection) — concluded CUE type system cannot express the structural constraint; Option A (import-time unwrap) is recommended

### Design Decisions Made
- **No Lisp/Zygomys rules** - Schema inference uses CUE-based detection, not a Lisp rules engine
- **No interactive review TUI** - Review workflow removed; `pudl schema reinfer` handles batch re-inference
- **Schema inference via CUE** - Heuristics + CUE unification for automatic schema detection
- **Schema name normalization** - Canonical `<package>.<#Definition>` format
- **Resource identity** - Stable `resource_id` from schema + identity fields; catchall uses content hash
- **Content hash dedup** - Universal dedup gate: if hash matches, skip regardless of schema
- **Collections are provenance** - Resources own identity independent of collection

## Future Development

### Phase 1: Analytical Layer (Next Priority)
The single most impactful work is building features that turn PUDL from "a place data goes" into "a tool that tells me things." Resource identity tracking is now in place as the prerequisite.

1. **`pudl diff`** - Compare two versions of the same resource (resource_id + version)
2. **`pudl summary`/`pudl stats`** - Aggregate views ("47 EC2 instances, 3 outliers")
3. **Basic outlier detection** - Given N instances of a schema, identify unusual field values

### Phase 2: Schema Intelligence
1. **Two-tier schema system** - Broad type recognition + policy compliance
2. **Schema drift detection** - "This resource used to validate, now it doesn't"
3. **Schema coverage reports** - "37% of data matches a specific schema, 63% is generic"

### Phase 3: Correlation & Cross-Source
1. **Cross-source correlation** - Link AWS resources to K8s resources
2. **Temporal tracking** - Same resource across multiple imports (enabled by resource_id + version)
3. ~~**Resource identity**~~ - ✅ Implemented in identity tracking

### Phase 4: Advanced Analytics
1. **DuckDB/Parquet integration** - Analytical query engine for large datasets
2. **Expert system components** - Automatic detection of common substructures
3. **Dashboard/reporting interfaces** - Visual representation of infrastructure state

## Remaining Cut Candidates

These items were identified in the project review but not yet addressed:

- `op/` + `internal/cue/processor.go` + `cmd/process.go` - CUE custom function processor (unrelated to core purpose)
- `cmd/setup.go` - Shell integration (premature convenience optimization)
- `cmd/module.go` - Thin wrapper around `cue mod` commands
- ~~`internal/streaming/` - CDC-based streaming parser~~ — Fixed and working (2026-02-09)

## Core Packages

- `internal/importer/` - Data import logic
- `internal/identity/` - Resource identity extraction and computation
- `internal/schema/` - Schema loading and management
- `internal/validator/` - CUE validation, cascade validation, validation service
- `internal/inference/` - Schema inference engine
- `internal/schemaname/` - Schema name normalization
- `internal/schemagen/` - Schema generation from data
- `internal/database/` - SQLite catalog
- `internal/config/` - Configuration loading
- `internal/init/` - Repository initialization
- `internal/git/` - Git operations for schema repo
- `internal/idgen/` - Proquint ID generation
- `internal/errors/` - Error types
- `internal/ui/` - Output formatting
- `internal/doctor/` - Health checks
- `internal/lister/` - List/query operations
- `cmd/` - CLI command definitions
