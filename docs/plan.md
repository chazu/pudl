# PUDL Development Plan

Living document tracking what is built and what comes next.

## What's Built

The core pipeline is stable and tested. Execution-related features (models, methods, workflows, Glojure runtime, vault, artifacts) were implemented in Phases 1-8, then extracted into **mu** as a separate tool. What remains in pudl is the knowledge layer.

### Data Lake
- Multi-format import (JSON, YAML, CSV, NDJSON) with automatic format detection
- Collection support with wrapper detection and unwrapping
- SQLite catalog with query, filter, pagination, and provenance tracking
- Content-based identity (SHA256, proquint display) for deduplication
- Resource identity from schema identity fields with version tracking

### Schema System
- CUE-based schema inference using heuristics and CUE unification
- Schema generation from imported data (`pudl schema new`)
- Schema name normalization to canonical `<package>.#<Definition>` format
- Bootstrap schemas (`pudl/core.#Item`, `pudl/core.#Collection`)
- Git-backed schema repository with status/commit/log
- Pluggable type patterns (AWS, Kubernetes, GitLab)

### Validation and Verification
- CUE structural validation (`pudl validate`)
- Repository-wide validation (`pudl repo validate`)
- Fixed-point verification (`pudl verify`) confirming schema assignment stability

### Definitions
- Definition discovery via CUE schema reference patterns
- Dependency graph from cross-definition references with cycle detection
- Definition validation via CUE module loader
- CLI: `pudl definition list/show/validate/graph`

### Drift Detection
- JSON deep diff comparing declared vs catalog state
- Field-level diffing with added/removed/changed tracking
- Drift report storage and retrieval

### Catalog Layer
- Bootstrap `catalog.cue` registering core types
- `pudl catalog` browsing registered schema types
- Extensible by user-defined entries

### Mu Bridge
- `pudl export-actions` converts drift reports into mu-compatible JSON action specs
- Supports single-definition and `--all` modes

### Infrastructure
- `pudl repo init` creates `.pudl/` and installs Claude skills
- `pudl doctor` with directory structure validation
- Database migrations (idempotent, run on every open)

## What's Next

Potential future work, roughly ordered by value.

### Richer Catalog
- Schema coverage reports (what percentage of data matches specific schemas)
- Catalog-driven code generation and documentation
- Cross-source correlation (linking AWS resources to Kubernetes resources)
- Temporal tracking (same resource across imports via `resource_id` + `version`)

### Deeper Mu Integration
- Bidirectional protocol: mu queries pudl for context during execution
- Action result feedback: mu reports outcomes back to pudl
- Richer action types beyond field-level drift (create, delete, reconcile)

### More Type Patterns
- Azure, GCP, Terraform state files
- Docker Compose, Helm values, CI/CD pipeline configs
- User-defined pattern registration

### Analytics
- `pudl diff` to compare two versions of the same resource
- `pudl summary` / `pudl stats` for aggregate views
- Basic outlier detection across instances of a schema
- DuckDB/Parquet integration for analytical queries on large datasets

### UI Improvements
- Dashboard/reporting for catalog and drift state
- Interactive TUI for browsing catalog entries and schemas

## Core Packages

| Package | Path | Responsibility |
|---------|------|----------------|
| `importer` | `internal/importer/` | Import pipeline, format detection, collections, wrapper detection |
| `inference` | `internal/inference/` | Schema inference (heuristics + CUE unification) |
| `identity` | `internal/identity/` | Resource identity extraction and computation |
| `idgen` | `internal/idgen/` | Content IDs, SHA256, proquint encoding |
| `database` | `internal/database/` | SQLite catalog CRUD and queries |
| `validator` | `internal/validator/` | CUE validation |
| `definition` | `internal/definition/` | Definition loader, validator, dependency graph |
| `drift` | `internal/drift/` | State comparator, report store |
| `mubridge` | `internal/mubridge/` | Drift-to-mu action export |
| `schemaname` | `internal/schemaname/` | Schema name normalization |
| `schemagen` | `internal/schemagen/` | Schema generation from data |
| `typepattern` | `internal/typepattern/` | Pluggable type detection patterns |
| `streaming` | `internal/streaming/` | CDC chunkers, format processors |
| `config` | `internal/config/` | YAML configuration |
| `init` | `internal/init/` | Workspace initialization |
| `git` | `internal/git/` | Git operations on schema repo |
| `lister` | `internal/lister/` | List/query with filters |
| `doctor` | `internal/doctor/` | Health checks, directory validation |
| `schema` | `internal/schema/` | Schema operations |
| `repo` | `internal/repo/` | Repo init, skill installation |
| `skills` | `internal/skills/` | Embedded Claude skill files |
| `vault` | `internal/vault/` | Vault interface (retained for definition resolution) |
| `ui` | `internal/ui/` | Output formatting |
| `errors` | `internal/errors/` | Typed error codes |
| `cmd` | `cmd/` | CLI command definitions (Cobra) |
