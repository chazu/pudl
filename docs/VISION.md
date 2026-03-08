# PUDL Vision & Architecture

This document describes what PUDL is today and where it's headed.

## Core Purpose

PUDL is a **personal infrastructure knowledge base and data lake** designed to help SRE/platform engineers/software engineers navigate, learn about, and isolate issues in their cloud environments. The long-term goal is **outlier detection and sprawl reduction** in cloud infrastructure and configuration.

## What Exists Today

### Data Lake Structure
- **Schema Repository**: `~/.pudl/schema/` - Git repository containing CUE schema definitions
- **Data Storage**: `~/.pudl/data/` - Imported data files with full provenance metadata
- **Catalog Database**: SQLite-based catalog with query, filter, and pagination support
- **Self-contained**: No external dependencies required for PUDL binary operation

### Schema System
- **CUE Lang**: Schema definition, validation, and constraint checking
- **Cascade Validation**: Multi-level schema matching (specific -> fallback -> catchall)
- **Schema Inference**: Automatic CUE-based schema detection using heuristics and CUE unification
- **Schema Generation**: `pudl schema new` generates CUE schemas from imported data
- **Schema Name Normalization**: Canonical format for consistent schema references
- **Git Integration**: `pudl schema status/commit/log` for version-controlled schemas

### Data Management
- **Multi-format Import**: JSON, YAML, CSV, NDJSON with automatic format detection
- **Collection Support**: Collections split into individual items with parent references
- **Provenance Tracking**: Timestamp, origin, format, and schema assignment tracked per entry
- **Content-based Identity**: SHA256 content hashing with proquint display format for resource deduplication
- **Export**: Multi-format output support

### Models & Definitions
- **Model Discovery**: Automatic discovery of models in the schema repository with schema reference resolution
- **Method & Socket Extraction**: Models declare methods (operations) and sockets (typed connection points)
- **Definition Loader**: Load, parse, and validate definitions that wire models together
- **Socket Wiring & Dependency Graph**: Definitions bind sockets across models; dependency graph built with cycle detection and topological sort
- **Repository Validation**: Workspace-wide validation across schemas, models, and definitions

### Method Execution
- **Glojure Runtime**: Embedded Clojure-like scripting language for method implementations
- **Lifecycle Dispatch**: Three-phase execution: qualification, action, post-action
- **CUE Function Unification**: CUE-based function definitions processed and unified with Glojure implementations
- **Builtin Namespaces**: `pudl.core` (string ops, data transforms) and `pudl.http` (HTTP requests) available out of the box

### Artifact Management
- **Method Outputs as Data**: Method execution results stored alongside imported data in the catalog
- **Content Hashing & Dedup**: Artifacts receive content-based IDs, same as imported data
- **Artifact Metadata**: Each artifact tracks its source definition, method, run_id, and tags

### Vault System
- **Credential Management**: Secure storage for secrets used by methods and workflows
- **Multiple Backends**: Environment variable backend and age-encrypted file backend
- **Runtime Resolution**: `vault://` references in definitions and methods resolved at execution time
- **Key Rotation**: Rotate encryption keys for file-backed vault entries

### Workflow Orchestration
- **CUE-based DAG Definition**: Workflows defined in CUE with steps, dependencies, and parameters
- **Automatic Dependency Resolution**: Dependencies inferred from field references between steps
- **Concurrent Execution**: Independent steps run concurrently; dependent steps wait for prerequisites
- **Run Manifests**: Each workflow run produces a manifest recording inputs, outputs, timing, and status

### Drift Detection
- **Declared vs Live State**: Compare what definitions declare against actual live state
- **JSON Deep Diff**: Field-level diffing with added/removed/changed tracking
- **Drift Reports**: Reports stored in `.drift/` directory for historical tracking
- **Refresh Mode**: Re-fetch live state before comparison with `--refresh`

### Agent Integration
- **Effect Types**: Audit trail effects and dry-run mode for safe agent-driven operations
- **Skill Files**: Declarative skill definitions for agent capabilities
- **Model Search & Scaffold**: Agents can search for models and scaffold new definitions
- **Extension Model Discovery**: Discover and integrate models from extension repositories

### CLI Commands
- `pudl init` - Initialize the data lake
- `pudl setup` - Set up shell integration (aliases, functions)
- `pudl config` - View and manage configuration
- `pudl import` - Import data files
- `pudl list` - Query and filter catalog entries
- `pudl show` - Show details of a catalog entry
- `pudl export` - Export data in various formats
- `pudl delete` - Remove catalog entries
- `pudl validate` - Validate data against schemas
- `pudl schema *` - Full schema lifecycle (list, add, new, show, edit, reinfer, migrate, generate-type, status, commit, log)
- `pudl model list` - List discovered models
- `pudl model show` - Show model details (methods, sockets, schema refs)
- `pudl model search` - Search models by name, tag, or capability
- `pudl model scaffold` - Generate a new model skeleton
- `pudl definition list` - List definitions
- `pudl definition show` - Show definition details and socket bindings
- `pudl definition validate` - Validate a definition against its model
- `pudl definition graph` - Display dependency graph
- `pudl repo validate` - Workspace-wide validation of schemas, models, and definitions
- `pudl method run` - Execute a method with lifecycle dispatch
- `pudl method list` - List available methods
- `pudl data search` - Search artifacts and imported data
- `pudl data latest` - Show the most recent data for a schema/source
- `pudl vault get` - Retrieve a secret
- `pudl vault set` - Store a secret
- `pudl vault list` - List stored secrets
- `pudl vault rotate-key` - Rotate vault encryption key
- `pudl workflow run` - Execute a workflow DAG
- `pudl workflow list` - List defined workflows
- `pudl workflow show` - Show workflow details and steps
- `pudl workflow validate` - Validate workflow definitions
- `pudl workflow history` - Show past workflow runs
- `pudl drift check` - Compare declared vs live state (--all, --refresh)
- `pudl drift report` - View stored drift reports
- `pudl process` - Process a CUE file with custom functions
- `pudl module` - Manage CUE module dependencies (tidy, list, info)
- `pudl migrate` - Run database migrations
- `pudl doctor` - Health check utility
- `pudl completion` - Generate shell completion scripts

### Technology Stack
- **Go** -- Core application with Cobra CLI framework
- **CUE Lang** -- Schema definition and validation
- **SQLite** -- Catalog database
- **Glojure** -- Clojure-like scripting for methods
- **age** -- Encryption for file vault

## Future Vision

The following features are aspirational and not yet implemented.

### Analytics Layer
- **Diff**: Compare two imports of the same resource type, show what changed
- **Summary/Stats**: Aggregate views ("47 EC2 instances, 3 outliers")
- **Basic Outlier Detection**: Given N instances of a schema, identify unusual field values
- These features transform PUDL from "a place data goes" into "a tool that tells me things"

### Schema Intelligence
- **Two-Tier Schema System**: Broad type recognition + policy compliance
  - Nothing rejected if it's a valid instance of the resource type
  - Easy identification of policy violations/outliers
  - Enables infrastructure standardization efforts
- **Schema Drift Detection**: "This resource used to validate, now it doesn't"
- **Schema Coverage Reports**: "37% of data matches a specific schema, 63% is generic"

### Correlation & Cross-Source
- **Cross-Source Correlation**: Link AWS resources to K8s resources
- **Temporal Tracking**: Same resource across multiple imports
- **Resource Linking**: Connect related resources across different sources and schemas

### Advanced Analytics
- **DuckDB/Parquet Integration**: Analytical query engine for large datasets
- **Expert System Components**: Automatic detection of common substructures
- **Dashboard/Reporting Interfaces**: Visual representation of infrastructure state

## Implementation Philosophy

- **Small incremental steps** to avoid large rework cycles
- **Keep doors open** for future capabilities without over-engineering
- **User-friendly workflows** with clear CLI output
- **Minimal viable features** before expanding scope

## Open Questions

1. **Schema Drift Handling**: Hard validation vs. soft warnings vs. automatic evolution
2. **Correlation Timing**: Ingestion-time vs. on-demand computation
3. **Data Partitioning Strategy**: Optimal partitioning for analytical queries
