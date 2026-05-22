# PUDL Features

What pudl can do today. Each section links to deeper docs where available.

Designations:
- **CORE** — Stable, well-tested, load-bearing
- **IN PROGRESS** — Functional but with known gaps or incomplete wiring
- **EXPERIMENTAL** — Early or lightly integrated; API may change

## Data Import — CORE

- Multi-format import: JSON, YAML, CSV, NDJSON (`pudl import --path`)
- Automatic format detection
- Content-based deduplication via SHA256 → proquint IDs
- Streaming parser with backpressure and memory monitoring
- Collection support: NDJSON files and wrapped JSON arrays are split into individual items with a parent collection entry
- Wrapper detection: identifies common API response wrappers (e.g. `{"items": [...]}`) and unwraps automatically
- Envelope JSON: structured import from mu plugins with embedded schema refs
- Reclassification of items with unresolved schema refs (`pudl reclassify`)

See [collections.md](docs/collections.md), [concepts.md](docs/concepts.md)

## Schema System — CORE

- CUE-based schema repository, git-tracked at `~/.pudl/schemas/`
- Schema inference: heuristic scoring + CUE unification to match data to schemas
- Inheritance graph for schema relationships
- Schema generation from imported data (`pudl schema new`)
- Schema name normalization to canonical `package.#Definition` format
- Bootstrap schemas: `pudl/core.#Item`, `pudl/core.#Collection`
- Pluggable type patterns for AWS, Kubernetes, GitLab (23+ patterns with confidence scoring)
- Schema CRUD: `list`, `show`, `add`, `edit`
- Version control: `status`, `commit`, `log`
- Bulk operations: `reinfer`, `migrate`
- `_pudl` metadata block in CUE schemas for identity fields, tracked fields, resource type

See [schema-authoring.md](docs/schema-authoring.md), [inference-algorithm.md](docs/inference-algorithm.md)

## Catalog — CORE

- SQLite catalog at `~/.pudl/data/sqlite/catalog.db`
- List, show, delete, export entries
- Filter by schema, origin, source, format, date range
- Pagination and sorting
- JSON output mode (`--json`) on all commands
- Resource identity tracking from schema-declared identity fields
- Version tracking for same-identity resources across imports
- Bootstrap `catalog.cue` for registered types (`pudl catalog`)

See [architecture.md](docs/architecture.md)

## Validation & Verification — CORE

- CUE structural validation against assigned schemas (`pudl validate`)
- Repository-wide validation (`pudl repo validate`)
- Fixed-point verification: confirms schema inference is stable (`pudl verify`)
- Cascade validation: intended schema → base schema → catchall fallback via CUE unification

## Definitions — CORE

- Named instances of schemas discovered from CUE files
- Dependency graph with cycle detection (`pudl definition graph`)
- Definition validation against schema interfaces (`pudl definition validate`)
- Interface checking: verifies definitions conform to schema constraints
- Multi-path discovery with per-repo workspace shadowing

## Drift Detection — CORE

- JSON deep diff comparing declared (definition) vs actual (catalog) state
- Field-level diffing: added, removed, changed fields
- Drift report storage and retrieval (`pudl drift check`, `pudl drift report`)
- Convergence status tracking per definition: unknown → clean → drifted → converging → converged → failed (`pudl status`)

## Mu Integration — CORE

- Export drift reports as mu-compatible action specs (`pudl export-actions`)
- BRICK-aware: reads `brick.#Target` toolchain and config from definitions
- Ingest mu build manifests (`pudl ingest-manifest`)
- Ingest mu observe results as live state for drift detection (`pudl ingest-observe`)
- Envelope JSON import with schema sidecar from mu plugins
- Schema cache for mu plugin output

See [mu-integration.md](docs/mu-integration.md)

## Bitemporal Fact Store — CORE

- General-purpose fact persistence with valid-time and transaction-time dimensions
- Four temporal query modes: as-of-now, as-of-valid, as-of-transaction, bitemporal
- Content-addressed fact IDs
- Fact lifecycle: assert, retract, invalidate
- CLI: `pudl facts list/show/retract/invalidate`
- Serves as EDB source for Datalog evaluator

See [facts.md](docs/facts.md)

## Datalog Evaluator — CORE

- Semi-naive bottom-up evaluation to fixed point
- Multi-source EDB: fact store + catalog
- Rule loading from CUE files
- Rule installation into workspace (`pudl rule add`)
- Query derived facts (`pudl query <relation>`)

See [datalog.md](docs/datalog.md)

## Per-Repo Workspaces — IN PROGRESS

- `pudl repo init` creates `.pudl/workspace.cue` with local `schema/` and `definitions/` directories
- Workspace discovery walks up from cwd to find `.pudl/workspace.cue`
- Catalog queries scoped by workspace origin (bypass with `--all-workspaces`)
- Per-repo schema and definition paths shadow global paths
- Imports within a workspace auto-tagged with workspace name

Known gap: workspace context infrastructure exists but most CLI commands still use only the global schema path. Per-repo `definitions/` and `schema/` directories are not yet wired into definition, drift, export-actions, or schema commands. See [workspace-context-cli-wiring.md](docs/issues/workspace-context-cli-wiring.md).

## Observations — EXPERIMENTAL

- Record structured observations about the codebase (`pudl observe`)
- Observation kinds: fact, obstacle, pattern, antipattern, suggestion, bug, opportunity
- Scoped to repo:path (e.g. `pudl:internal/database`)
- Stored as facts in the bitemporal store with dedup

## CUE Module Management — CORE

- `pudl module tidy` — fetch and update dependencies
- `pudl module list` — list current dependencies
- `pudl module info` — show module information
- `pudl module add` — add third-party module dependency

## Developer & Agent Tools — CORE

- `pudl doctor` — workspace health checks (directory structure, database integrity, schema repo, git)
- `pudl prime` — output structured prompt teaching agents how to use pudl
- `pudl repo init` installs Claude skill files into `.pudl/`
- `pudl version` — version info
- `pudl config set/reset` — configuration management
- `pudl setup` — shell integration

## Output & UI — IN PROGRESS

- All commands support `--json` for machine-readable output — CORE
- Table, JSON, YAML output formats — CORE
- Interactive list UI (bubbletea-based) for browsing catalog entries — EXPERIMENTAL (single integration point: `pudl list`)
