# PUDL - Development Plan

## Current State

The core import/catalog/schema pipeline is stable and well-tested (291+ passing tests). All features below are implemented and working.

### What's Built

- **Data Import** — JSON, YAML, CSV, NDJSON with automatic format detection
- **Collection Support** — NDJSON and JSON API wrapper detection/unwrapping
- **SQLite Catalog** — Query, filter, pagination, and provenance tracking
- **CUE Schema Inference** — Heuristic scoring + CUE unification (no hard-coded rules)
- **Cascade Validation** — Multi-level schema matching (specific → fallback → catchall)
- **Schema Generation** — `pudl schema new` generates CUE schemas from imported data
- **Resource Identity** — Content-hash dedup, stable `resource_id`, version tracking
- **Schema Name Normalization** — Canonical `<package-path>.#<Definition>` format
- **Bootstrap Schemas** — Embedded CUE files (`pudl/core.#Item`, `pudl/core.#Collection`)
- **Full CLI** — Import, list, show, delete, export, validate, schema lifecycle, doctor

### Key Design Decisions

- **CUE-based inference** — Heuristics + CUE unification; no Lisp/Zygomys rules engine
- **No interactive review TUI** — `pudl schema reinfer` handles batch re-inference
- **Schema name normalization** — Canonical `<package>.<#Definition>` format
- **Content hash dedup** — Universal dedup gate: if hash matches, skip regardless of schema
- **Resource identity** — Stable `resource_id` from schema + identity fields; catchall uses content hash
- **Collections are provenance** — Resources own identity independent of collection

## Roadmap

### Phase 1: Analytical Layer (Next Priority)

The single most impactful work: turn PUDL from "a place data goes" into "a tool that tells me things." Resource identity tracking is the prerequisite — already implemented.

1. **`pudl diff`** — Compare two versions of the same resource (`resource_id` + `version`)
2. **`pudl summary` / `pudl stats`** — Aggregate views ("47 EC2 instances, 3 outliers")
3. **Basic outlier detection** — Given N instances of a schema, identify unusual field values

### Phase 2: Schema Intelligence

1. **Two-tier schema system** — Broad type recognition + policy compliance
2. **Schema drift detection** — "This resource used to validate, now it doesn't"
3. **Schema coverage reports** — "37% of data matches a specific schema, 63% is generic"

### Phase 3: Correlation & Cross-Source

1. **Cross-source correlation** — Link AWS resources to K8s resources
2. **Temporal tracking** — Same resource across multiple imports (enabled by `resource_id` + `version`)

### Phase 4: Advanced Analytics

1. **DuckDB/Parquet integration** — Analytical query engine for large datasets
2. **Expert system components** — Automatic detection of common substructures
3. **Dashboard/reporting interfaces** — Visual representation of infrastructure state

## Cut Candidates

Identified in project review but not yet addressed:

- `op/` + `internal/cue/processor.go` + `cmd/process.go` — CUE custom function processor (unrelated to core purpose)
- `cmd/setup.go` — Shell integration (premature convenience optimization)
- `cmd/module.go` — Thin wrapper around `cue mod` commands

## Completed Work Log

Detailed implementation history is in the [`implog/`](../implog/) directory. Key milestones:

| Date | Work |
|------|------|
| 2025-08 | CLI foundation, workspace init, data import, catalog, listing |
| 2025-11 | Schema inference refactor (removed hard-coded rules → CUE-based) |
| 2026-01-29 | Codebase cleanup, CUE module consolidation |
| 2026-02-04 | Schema generation improvements |
| 2026-02-05 | Schema name normalization |
| 2026-02-06 | Resource identity tracking, codebase cleanup |
| 2026-02-09 | Streaming parser fixes, CDC EOF handling |
| 2026-02-13 | Schema generate-type command, type detection |
| 2026-02-18 | Collection wrapper detection research |
| 2026-03 | Collection wrapper detection + unwrap implementation |

## Core Packages

| Package | Path | Responsibility |
|---------|------|----------------|
| `importer` | `internal/importer/` | Import pipeline, format detection, collections, wrapper detection |
| `inference` | `internal/inference/` | Schema inference (heuristics + CUE unification) |
| `identity` | `internal/identity/` | Resource identity extraction and computation |
| `idgen` | `internal/idgen/` | Content IDs, SHA256, proquint encoding |
| `database` | `internal/database/` | SQLite catalog CRUD and queries |
| `validator` | `internal/validator/` | CUE validation, cascade validation |
| `schemaname` | `internal/schemaname/` | Schema name normalization |
| `schemagen` | `internal/schemagen/` | Schema generation from data |
| `streaming` | `internal/streaming/` | CDC chunkers, format processors |
| `config` | `internal/config/` | YAML configuration |
| `init` | `internal/init/` | Workspace initialization |
| `git` | `internal/git/` | Git operations on schema repo |
| `lister` | `internal/lister/` | List/query with filters |
| `ui` | `internal/ui/` | Output formatting, interactive TUI |
| `doctor` | `internal/doctor/` | Health checks |
| `errors` | `internal/errors/` | Typed error codes |
| `cmd` | `cmd/` | CLI command definitions (Cobra) |
