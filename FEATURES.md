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
- Fixed-point verification: confirms schema inference is stable (`pudl verify`)
- Cascade validation: intended schema → base schema → catchall fallback via CUE unification

## System Models — CORE

- `#SystemModel` instances declare a system's desired state as a set of `desired` entries (each entry is a "definition")
- Model registry with last-run status (`pudl model list`)
- Inspect a model's desired entries and details (`pudl model show <name>`)
- Validate a model against its schema (`pudl model validate <name>`)
- Models discovered from CUE files with per-repo workspace shadowing

## Run & Drift — CORE

- `pudl run <name>` drives the observe-only ACUTE loop: populate → drift → checks → report
- Drift is a phase of `pudl run`: JSON deep diff comparing declared desired vs observed (catalog) state, with field-level diffing (added, removed, changed)
- `pudl run <name> --converge` closes drift: pudl renders desired → sources and the mu plugin reconciles
- Convergence status tracking per model instance: unknown → clean → drifted → converging → converged → failed; a run records its verdict on the instance row, read back via `pudl status`

## Mu Integration — CORE

- pudl declares desired/observed state; mu executes — pudl shells out to mu, with no execution layer of its own
- Ingest mu build manifests (`pudl mu ingest-manifest`)
- Ingest mu observe results as live state for drift detection (`pudl mu ingest-observe`)
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

Known gap: workspace context infrastructure exists but most CLI commands still use only the global schema path. Per-repo `models/` and `schema/` directories are not yet wired into model, run, or schema commands. See [workspace-context-cli-wiring.md](docs/issues/workspace-context-cli-wiring.md).

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
