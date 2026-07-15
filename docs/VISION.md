# PUDL Vision & Architecture

This document describes what PUDL is today and where it's headed.

## Core Purpose

PUDL is a **personal data lake that knows things**. It ingests structured data (JSON, YAML, CSV, NDJSON), infers schemas using CUE, validates entries, tracks drift, and maintains a catalog of everything it has seen. The long-term goal is **outlier detection and sprawl reduction** in cloud infrastructure and configuration.

PUDL is the knowledge layer. It tells you what exists, what shape it has, what changed, and what's wrong. It does not execute remediation -- that is the job of **mu** (a separate tool at `~/dev/go/mu/`).

### The Split: pudl knows, mu acts

- **pudl** detects drift, validates schemas, catalogs data, and renders desired state.
- **mu** receives desired-state sources and executes them (plugin protocol, effect dispatch).
- `pudl run --converge` bridges the two: it renders the selected model and ingests
  mu's observe and manifest results.

This separation keeps pudl focused on data and knowledge while mu handles side effects and execution.

## What Exists Today

### Data Lake Structure
- **Schema Repository**: `~/.pudl/schema/` -- Git repository containing CUE schema definitions
- **Data Storage**: `~/.pudl/data/` -- Imported data files with full provenance metadata
- **Catalog Database**: SQLite-based catalog with query, filter, and pagination support
- **Bitemporal Fact Store**: General-purpose table for typed assertions with valid-time and transaction-time tracking, content-addressed dedup, and four temporal query modes (see [docs/facts.md](facts.md))
- **Self-contained**: No external dependencies required for the PUDL binary

### Schema System
- **CUE Lang**: Schema definition, validation, and constraint checking
- **Schema Inference**: Automatic CUE-based schema detection using heuristics and CUE unification
- **Schema Generation**: `pudl schema new` generates CUE schemas from imported data
- **Schema Name Normalization**: Canonical `<package>.#<Definition>` format
- **Git Integration**: `pudl schema status/commit/log` for version-controlled schemas
- **Type Patterns**: Pluggable type detection patterns (AWS, Kubernetes, GitLab, etc.)
- **Bootstrap Schemas**: Embedded CUE files (`pudl/core.#Item`, `pudl/core.#Collection`)

### Data Management
- **Multi-format Import**: JSON, YAML, CSV, NDJSON with automatic format detection
- **Collection Support**: NDJSON collections split into individual items with normalized memberships; typed envelopes preserve schema metadata around payloads
- **Provenance Tracking**: Timestamp, origin, format, and schema assignment tracked per entry
- **Content-based Identity**: SHA256 content hashing with proquint display format for resource deduplication
- **Resource Identity**: Stable `resource_id` from schema identity fields; version tracking
- **Export**: Multi-format output support

### System Models
- **Model Loading**: `#SystemModel` instances resolve from workspace-first CUE schemas
- **Desired Resources**: Models declare concrete resources to observe, compare, and optionally converge
- **Dependency Graph**: Model `depends_on` declarations become queryable bitemporal facts
- **Validation**: `pudl model validate` checks model structure before a run

### Catalog Layer
- **Bootstrap `catalog.cue`**: Defines `#CatalogEntry` and registers core types
- **`pudl catalog`**: Lists all registered schema types with metadata
- **Extensible**: Users add their own catalog entries

### Drift Detection
- **Desired vs Observed State**: Compare model resources against live observations or a current catalog snapshot
- **JSON Deep Diff**: Field-level diffing with added/removed/changed tracking
- **Run Reports**: Verdicts and reports are recorded with the model run and catalog entries

### Fixed-Point Verification
- **`pudl verify`**: Re-runs inference on all catalog entries and confirms schema assignments are stable
- **Schema Stability**: Detects when schema changes cause existing data to be classified differently

### Mu Bridge
- **Observe snapshots**: `pudl mu ingest-observe` stores a timestamped collection
- **Build manifests**: `pudl mu ingest-manifest` records per-action results
- **Convergence**: `pudl run --converge` renders desired state and delegates mutation to mu

### Structural Validation
- **`pudl validate`**: Validates data against CUE schemas
- **Workspace Resolution**: Schema and model commands search project-local CUE before global CUE

### CLI Commands
- `pudl init` -- Initialize the data lake
- `pudl setup` -- Set up shell integration
- `pudl config` -- View and manage configuration
- `pudl import` -- Import data files
- `pudl list` -- Query and filter catalog entries
- `pudl show` -- Show details of a catalog entry
- `pudl export` -- Export data in various formats
- `pudl delete` -- Remove catalog entries
- `pudl validate` -- Validate data against schemas
- `pudl verify` -- Fixed-point verification of schema stability
- `pudl catalog` -- Browse registered schema types
- `pudl schema *` -- Full schema lifecycle (list, add, new, show, edit, reinfer, migrate, generate-type, status, commit, log)
- `pudl model list/show/validate` -- Inspect registered system models
- `pudl run` -- Populate, detect drift, check, report, and optionally converge
- `pudl status` -- Read recorded model/resource convergence status
- `pudl repo init` -- Initialize a repo with pudl config and Claude skills
- `pudl model validate` -- Validate a system model against its schema
- `pudl mu ingest-observe` -- Ingest observe results and create a snapshot
- `pudl mu ingest-manifest` -- Ingest mu build manifests
- `pudl module` -- Manage CUE module dependencies
- `pudl migrate` -- Run database migrations
- `pudl observe` -- Record structured observations about the codebase
- `pudl facts list/show/retract/invalidate` -- Manage facts in the bitemporal store
- `pudl query` -- Evaluate Datalog rules and query derived facts
- `pudl rule add` -- Validate and install Datalog rule files
- `pudl doctor` -- Health check utility
- `pudl completion` -- Generate shell completion scripts

### Agent Observations
- **`pudl observe`**: Agents and humans record structured observations, stored as facts in the bitemporal store
- **Observation schema**: `pudl/nous.#Observation` with kind taxonomy (fact, obstacle, pattern, antipattern, suggestion, bug, opportunity)
- **Corroboration**: Multiple agents independently flagging the same thing produces distinct facts; the count is signal

### Datalog Evaluator
- **`pudl query`**: Semi-naive bottom-up evaluation over facts and catalog entries as EDB
- **CUE-defined rules**: `#Rule` values with head/body structure, `$`-prefixed variables, stored in workspace-scoped rule directories
- **`pudl rule add`**: Validates and installs rule files with workspace scoping (repo-scoped shadows global)
- **Hash-indexed joins**: O(1) lookup for bound variables and ground terms during rule evaluation

### Technology Stack
- **Go** -- Core application with Cobra CLI framework
- **CUE Lang** -- Schema definition, validation, inference, and Datalog rule syntax
- **SQLite** -- Catalog database and bitemporal fact store

## Future Vision

### Observation Promotion Pipeline
- **Worth tracking**: Observations gain/lose worth based on corroboration, contradiction, and decay
- **`pudl promote`**: Convert validated observations into Datalog rules or conventions
- **Human review gate**: Candidates from nous enter review before promotion to stable knowledge

### nous Integration
- **nous reads from pudl**: Unit store hydrated from catalog entries and derived facts (IDB)
- **nous writes to pudl**: Discovered patterns, conjectures, and candidate rules stored as facts
- **Three-loop architecture**: Fast (Datalog inference) → Medium (nous agenda) → Slow (human validation)

### Deeper CUE Integration
- **Catalog-driven generation**: Use the schema catalog to drive code generation, documentation, and tooling
- **Fixed-point properties**: Extend `pudl verify` to check broader invariants (e.g., all definitions resolve, all schemas have at least one matching entry)
- **Richer constraints**: Policy-tier schemas for compliance checking layered on top of base schemas

### Richer Mu Plugin Protocol
- **Bidirectional communication**: mu queries pudl for context during execution
- **Action result feedback**: mu reports execution outcomes back to pudl for catalog update
- **Typed action specs**: Richer action types beyond field-level drift (create, delete, reconcile)

### Structural Validation
- **Cross-source correlation**: Link resources across providers (AWS + Kubernetes)
- **Temporal tracking**: Same resource across multiple imports using `resource_id` + `version`
- **Schema coverage reports**: Distribution of schema matches across the catalog

### Analytics
- **Diff**: Compare two imports of the same resource type, show what changed
- **Summary/Stats**: Aggregate views ("47 EC2 instances, 3 outliers")
- **Basic outlier detection**: Given N instances of a schema, identify unusual field values
- **DuckDB/Parquet integration**: Analytical query engine for large datasets

### More Type Patterns
- **Broader cloud coverage**: Azure, GCP, Terraform state files
- **Application config**: Docker Compose, Helm values, CI/CD pipeline configs

### UI Improvements
- **Dashboard/reporting**: Visual representation of catalog and drift state
- **Interactive browsing**: TUI for navigating catalog entries and schemas

## Implementation Philosophy

- **Small incremental steps** to avoid large rework cycles
- **Keep doors open** for future capabilities without over-engineering
- **pudl knows, mu acts** -- maintain the separation of knowledge and execution
- **CUE as the backbone** -- schema inference, validation, and definitions all flow through CUE
