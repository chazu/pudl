# PUDL Development Plan

Living document tracking what is built and what comes next.

## What's Built

The core pipeline is stable and tested. Execution-related features (models, methods, workflows, Glojure runtime, artifacts) were implemented in Phases 1-8, then extracted into **mu** as a separate tool. What remains in pudl is the knowledge layer. Residual execution CLI surface (`pudl data search/latest`, `drift check --method`, Glojure adapter) has been removed; some internal artifacts (database fields, CUE model schemas) remain for future cleanup.

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
- BRICK-aware: `brick.#Target` toolchain and config fields used directly

### ACUTE Feedback Loop
- `pudl ingest-observe` — ingest mu observe results as live state for drift detection
- `pudl ingest-manifest` — ingest mu build manifests, track per-action results
- `pudl status` — per-definition convergence status (unknown/clean/drifted/converging/converged/failed)
- Status column on catalog entries, updated through the full ACUTE cycle
- Architecture: [`docs/acute-loop-architecture.md`](acute-loop-architecture.md)

### Per-Repo Workspaces
- `pudl repo init` creates `.pudl/workspace.cue` with schema/ and definitions/ directories
- Workspace discovery walks up from cwd looking for `.pudl/workspace.cue`
- Catalog queries scoped by workspace origin (--all-workspaces to bypass)
- Multi-path definition discovery with per-repo shadowing of global definitions
- Multi-path schema resolution with per-repo shadowing of global schemas
- Imports within a workspace auto-tagged with workspace name as origin

### Agent Integration
- `pudl prime` outputs a structured prompt teaching agents how to use pudl
- `pudl guide` provides topic-based reference guides for agents and humans (overview, import, schemas, facts, datalog, definitions, drift, pith, mu, agents)
- `pudl repo init` creates `.pudl/` with workspace.cue and installs Claude skills

### Documentation
- Reorganized docs: user-facing guides in `docs/`, active work in `docs/beads/`, research in `docs/research/`, completed plans in `docs/archive/`
- `docs/README.md` index covers all subdirectories

### Infrastructure
- `pudl doctor` with directory structure validation
- Database migrations (idempotent, run on every open)

---

## Public API Extraction (fact store + datalog)

Goal: let external Go applications interact with pudl data stores (global `~/.pudl`
and repo-scoped `.pudl/`) through `pkg/factstore` and `pkg/eval` **without importing
`pudl/internal/*`**. The `internal/` rule already blocks external import of internal
packages; the work is making the `pkg/` facade complete and non-leaky.

### Phase 1 — Extraction + dead-code nuke (done)

The live query path (partition → SQL → recursive fallback) is currently inline in
`cmd/query.go` and not reusable. `pkg/eval` only exposes the legacy in-memory
evaluator, which is dead code. Plan:

1. `internal/datalog/match.go` — move shared helpers (`matchConstraints`,
   `valuesEqual`, `toFloat64`) out of `eval.go` before deletion (`sql_eval.go` uses
   `matchConstraints`).
2. `internal/datalog/query.go` — `Evaluate(db, rules, relation, constraints, scope)`,
   the single orchestrator; `cmd/query.go` calls it (behavior unchanged).
3. Delete dead code: `eval.go`, `eval_test.go`, `edb.go`, `index.go`; trim
   `Binding`/`Apply` from `types.go` (keep `ParseTerm` — loader uses it).
4. `pkg/eval` — strip to rules + types: `Rule/Atom/Term/Tuple/Var/Val`,
   `LoadRulesFromPaths`, `ParseRulesFromSource`. Remove `EDB`/`NewEvaluator`/
   `New*EDB`.
5. `pkg/factstore` — drop leaky `DB()`; add `QueryOptions` + `Store.Query` (calls
   `datalog.Evaluate`); re-export `Rule`/`Tuple` so query-only callers need only
   `factstore`.
6. `pkg/factstore/resolve.go` — `GlobalDir()`, `DiscoverWorkspace(cwd)` →
   `{RepoDir, RulePaths}`, wrapping `internal/config` + `internal/workspace` (no
   internal types in signatures).
7. Tests (factstore query covering SQL + recursive, eval parse, resolve), full
   suite green, implog.

Decisions locked: `Query` lives on `factstore.Store`; Phase 1 ships before Phase 2.

Done 2026-06-07 — see `implog/2026_06_07_public_api_extraction.md`. Also fixed a
latent recursion-routing bug in the query path: relations with both a base and a
recursive rule previously returned only the base tuples.

### Phase 2 — Catalog-as-datalog bridge (deferred)

Let datalog query the catalog alongside facts via one `Store.Query` API. Facts use a
JSON `args` blob; the catalog uses native columns — bridge with a SQL view that
projects `catalog_entries` into fact shape, wired through the compiler's existing
`CompileOptions.TableOverrides`. Must thread overrides through both `sql_eval.go` and
`recursive.go`; the view is atemporal so the override must win over the temporal
table swap. View column set deferred to Phase 2 start.

## What's Next

Potential future work, roughly ordered by value.

### Richer Catalog
- Schema coverage reports (what percentage of data matches specific schemas)
- Catalog-driven code generation and documentation
- Cross-source correlation (linking AWS resources to Kubernetes resources)
- Temporal tracking (same resource across imports via `resource_id` + `version`)

### Deeper Mu Integration
- Richer action types beyond field-level drift (create, delete, reconcile)

### More Type Patterns
- Azure, GCP, Terraform state files
- Docker Compose, Helm values, CI/CD pipeline configs
- User-defined pattern registration

### Analytics
- ~~`pudl summary` / `pudl stats` for aggregate views~~ → Done: `pudl facts stats --group-by`
- `pudl diff` to compare two versions of the same resource
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
| `mubridge` | `internal/mubridge/` | Drift-to-mu action export, manifest/observe ingestion |
| `workspace` | `internal/workspace/` | Per-repo workspace discovery, context resolution |
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
| `ui` | `internal/ui/` | Output formatting |
| `errors` | `internal/errors/` | Typed error codes |
| `cmd` | `cmd/` | CLI command definitions (Cobra) |
