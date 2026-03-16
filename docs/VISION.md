# PUDL Vision & Architecture

This document describes what PUDL is today and where it's headed.

## Core Purpose

PUDL is a **personal data lake that knows things**. It ingests structured data (JSON, YAML, CSV, NDJSON), infers schemas using CUE, validates entries, tracks drift, and maintains a catalog of everything it has seen. The long-term goal is **outlier detection and sprawl reduction** in cloud infrastructure and configuration.

PUDL is the knowledge layer. It tells you what exists, what shape it has, what changed, and what's wrong. It does not execute remediation -- that is the job of **mu** (a separate tool at `~/dev/go/mu/`).

### The Split: pudl knows, mu acts

- **pudl** detects drift, validates schemas, catalogs data, and exports action plans.
- **mu** receives those action plans and executes them (plugin protocol, effect dispatch).
- `pudl export-actions` bridges the two: it reads drift reports and emits mu-compatible JSON action specs.

This separation keeps pudl focused on data and knowledge while mu handles side effects and execution.

## What Exists Today

### Data Lake Structure
- **Schema Repository**: `~/.pudl/schema/` -- Git repository containing CUE schema definitions
- **Data Storage**: `~/.pudl/data/` -- Imported data files with full provenance metadata
- **Catalog Database**: SQLite-based catalog with query, filter, and pagination support
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
- **Collection Support**: Collections split into individual items with parent references; wrapper detection and unwrapping
- **Provenance Tracking**: Timestamp, origin, format, and schema assignment tracked per entry
- **Content-based Identity**: SHA256 content hashing with proquint display format for resource deduplication
- **Resource Identity**: Stable `resource_id` from schema identity fields; version tracking
- **Export**: Multi-format output support

### Definitions
- **Definition Discovery**: Text-based parsing to find definitions that unify against CUE schema types
- **Schema Reference Resolution**: Definitions reference schemas (not models) via CUE unification
- **Socket Wiring & Dependency Graph**: DAG built from cross-definition CUE references with cycle detection and topological sort
- **Validation**: Definitions validated via CUE module loader

### Catalog Layer
- **Bootstrap `catalog.cue`**: Defines `#CatalogEntry` and registers core types
- **`pudl catalog`**: Lists all registered schema types with metadata
- **Extensible**: Users add their own catalog entries

### Drift Detection
- **Declared vs Live State**: Compare what definitions declare against actual catalog state
- **JSON Deep Diff**: Field-level diffing with added/removed/changed tracking
- **Drift Reports**: Reports stored in `.drift/` directory for historical tracking

### Fixed-Point Verification
- **`pudl verify`**: Re-runs inference on all catalog entries and confirms schema assignments are stable
- **Schema Stability**: Detects when schema changes cause existing data to be classified differently

### Mu Bridge
- **`pudl export-actions`**: Reads drift reports and emits mu-compatible JSON action specs
- **Action Specs**: Each drift difference maps to a typed action with field, expected value, and actual value
- **Plan Response**: Single JSON document covering one or all definitions

### Structural Validation
- **`pudl validate`**: Validates data against CUE schemas
- **Repository Validation**: `pudl repo validate` checks schemas and definitions workspace-wide

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
- `pudl definition list` -- List definitions
- `pudl definition show` -- Show definition details
- `pudl definition validate` -- Validate a definition
- `pudl definition graph` -- Display dependency graph
- `pudl repo init` -- Initialize a repo with pudl config and Claude skills
- `pudl repo validate` -- Workspace-wide validation
- `pudl drift check` -- Compare declared vs live state (--all)
- `pudl drift report` -- View stored drift reports
- `pudl export-actions` -- Export drift as mu-compatible action specs
- `pudl module` -- Manage CUE module dependencies
- `pudl migrate` -- Run database migrations
- `pudl doctor` -- Health check utility
- `pudl completion` -- Generate shell completion scripts

### Technology Stack
- **Go** -- Core application with Cobra CLI framework
- **CUE Lang** -- Schema definition, validation, and inference
- **SQLite** -- Catalog database

## Future Vision

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
