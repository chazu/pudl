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

---

## Roadmap: Infrastructure Automation Expansion

PUDL is expanding from a data processing/cataloging tool into a full infrastructure automation system inspired by System Initiative. The existing data pipeline becomes the artifact management layer underneath a new execution engine. Nothing is removed — scope grows.

Reference: `architecture.docx` in repo root.

### Key Concepts: Schema vs Model vs Definition

| Concept | What it is |
|---------|-----------|
| **Schema** | A data shape. CUE constraints describing what a resource looks like. What pudl has today. |
| **Model** | A separate entity that *references* one or more schemas and adds methods, sockets, auth, metadata, and state shape. Models are not schemas — they compose schemas. |
| **Definition** | A named instance of a model with concrete args. A CUE value that unifies against a model's resource schema. |

Models reference schemas rather than embedding operational behavior into them. This preserves schema purity (a schema is always just a data shape) and enables reuse — the same schema can back multiple models, and schemas evolve independently of operational logic.

### Three Validation Layers

Validation in pudl operates at three distinct layers, each with a natural home:

| Layer | What it validates | How | Where it lives |
|-------|------------------|-----|----------------|
| **Base schema** | Structural shape — required fields, types, field constraints | CUE constraints | Schema files (existing pudl schemas) |
| **Policy schemas** | Stricter rules layered on top — compliance, security, business rules | CUE constraints (cascade unification) | Schema files (cascade validation tiers) |
| **Qualification methods** | Checks requiring runtime or external data — credential validity, resource existence, network reachability | Glojure methods with `kind: "qualification"` | Model method declarations + `.clj` files |

The key distinction: **can CUE validate it statically?**

- **Yes → it's a schema** (base or policy tier). Cascade validation handles the layering. Example: "replicas must be >= 3 in production" is a CUE constraint on a policy-tier schema.
- **No → it's a qualification method** on the model. Example: "does this AMI actually exist in AWS?" requires calling an API at runtime.

This means the existing two-tier schema system (base + policy/compliance) is preserved and orthogonal to models. A resource can have:
- A schema with policy tiers but **no model** (pure data validation, no operations)
- A model that references a base schema, and policy tiers still apply when validating definitions against it
- Qualification methods on a model that gate execution with runtime checks that CUE structurally cannot express

The three layers compose rather than replace each other.

### Methods

**Methods** have a `kind` field that determines their role in the execution lifecycle:

- `action` — CRUD and custom operations (default). Create, delete, restart, etc.
- `qualification` — Precondition check. Returns pass/fail. Declared to run before specific actions. These are aspects — cross-cutting concerns inserted at lifecycle cut-points.
- `attribute` — Computed value derivation. May overlap with native CUE expressions for simple cases; Glojure for cases requiring external data.
- `codegen` — Output transformation to other formats (JSON, YAML, HCL).

The runtime uses one method execution pipeline with aspect-like dispatch based on `kind` and lifecycle hooks (`before`, `blocks`).

**Sockets** are typed input/output ports on models, enabling inter-component data flow:

- Input sockets are like function arguments — what this component needs from others.
- Output sockets are like function returns — what this component provides to others.
- Socket types are CUE values (primitives or schemas).
- When definitions are connected via sockets, data flows automatically — a definition's output socket value populates another definition's input socket.
- Cross-definition CUE references are the file-based equivalent of socket wiring.

### Phase 1: Models — Composing Schemas with Behavior (COMPLETE)

**Goal:** Introduce models as a new concept separate from schemas. A model *references* one or more schemas and adds methods, sockets, authentication, and state shape. Schemas remain pure data shapes — unchanged from what pudl has today.

**Status:** Complete. See `implog/2026_03_06_phase1_models.md` for details.

Models live in their own directory (`models/`) and import schemas rather than extending them. This preserves schema reuse, independent evolution, and the existing cascade validation system.

1. **Define base types in bootstrap CUE:**
   - `#Method` — `kind` (action|qualification|attribute|codegen), `inputs`, `returns`, `timeout`, `retries`, `blocks` (for qualifications: which methods this gates)
   - `#Socket` — `direction` (input|output), `type` (CUE value/schema), `description`, `required`
   - `#ModelMetadata` — `name`, `description`, `category`, `icon`
   - `#AuthConfig` — `method` (bearer|sigv4|basic|custom), `credentials` (vault references)
   - `#QualificationResult` — `passed: bool`, `message: string` (standard return type for qualification methods)
   - `#Model` — top-level model type: `schema` (reference to a resource schema), `state` (optional state schema), `methods`, `sockets`, `auth`, `metadata`
2. **Model file convention** — Models are CUE files in `models/` that import and reference schemas:
   ```cue
   import "pudl/schemas/aws/ec2"

   #InstanceModel: #Model & {
       schema:   ec2.#Instance       // resource shape
       state:    ec2.#InstanceState   // live state shape (for drift)
       metadata: { name: "ec2_instance", category: "compute", ... }
       methods:  { list: ..., create: ..., delete: ... }
       sockets:  { vpc_id: { direction: "input", type: string }, ... }
       auth:     { method: "sigv4" }
   }
   ```
   Schemas (`ec2.#Instance`) stay in the schema repo unchanged. The model adds behavior around them.
3. **`pudl model list`** — List all models, showing which schema(s) each references
4. **`pudl model show <name>`** — Display model including referenced schemas, methods, sockets, auth, state shape
5. **Write 2-3 example models** to validate the format:
   - `aws/ec2.#InstanceModel` — references `ec2.#Instance` + `ec2.#InstanceState`, action methods (list, create, delete), qualification (valid_credentials, ami_exists), sockets (vpc_id input, instance_id output), sigv4 auth
   - `generic/http.#EndpointModel` — references `http.#Request` + `http.#Response`, action methods (get, post), basic/bearer auth, generic sockets
   - A simple model referencing a schema with no sockets, to verify minimal models work
   - Verify that schemas without models continue to work for pure data validation (cascade validation, import, etc.)

**Reuses:** CUE loader, validator, schema registry, bootstrap embed system. Schemas and cascade validation are untouched.

**New packages:** `internal/model/` (model discovery, schema reference resolution, method/socket extraction, lifecycle resolution — "given method X, what qualifications must run first?").

### Phase 2: Definitions — Named Resource Instances (COMPLETE)

**Goal:** Users declare named instances of models with concrete configuration and socket wiring.

**Status:** Complete. See `implog/2026_03_06_phase2_definitions.md` for details.

Definitions are CUE files in `~/.pudl/schema/definitions/` that unify against model schemas. The definition package provides discovery, validation, and dependency graph analysis based on socket wiring between definitions.

1. **Definition file convention** — `definitions/*.cue` files in the schema repository
2. **Definition discovery** — Text-based parsing to find definitions that unify against `#*Model` types
3. **Socket wiring** — Cross-definition CUE references detected and tracked as socket bindings
4. **Dependency graph** — DAG built from socket wiring with topological sort and cycle detection
5. **CLI commands** — `pudl definition list/show/validate/graph` and `pudl repo validate`
6. **Validation** — Definitions validated via CUE module loader (validates all CUE in workspace)

**Reuses:** CUE evaluator, cascade validator (for error reporting), schema name normalization.

**New packages:** `internal/definition/` (definition loader, validator, socket wiring resolution, dependency graph builder).

### Phase 3a: Glojure Runtime + CUE Function Unification (COMPLETE)

**Goal:** Embed the Glojure runtime and unify it with the existing CUE custom function system (`op/` + `internal/cue/processor.go`).

**Status:** Complete. See `implog/2026_03_07_phase3a_glojure_runtime.md` for details.

Glojure v0.6.4 embedded as Go dependency. Unified function registry (`internal/glojure/`) supports both Go and Glojure function implementations behind the `CustomFunction` interface. Two builtin namespaces registered: `pudl.core` (string ops, format, env, timestamps) and `pudl.http` (HTTP GET/POST/status/JSON). CUE processor upgraded with registry lookup, result caching, per-function timeouts, and improved error context.

PUDL has two execution layers that share one Glojure runtime but serve different purposes. CUE functions compute values during CUE evaluation (the "op layer"). Methods perform operations during `pudl method run` (the "execution layer"). See `docs/architecture.md` for the full rationale.

This phase establishes the shared runtime before methods exist, so methods build on an established foundation rather than introducing a parallel system.

1. **Embed the Glojure runtime** — `github.com/glojurelang/glojure` as a Go dependency
2. **Unified function registry** — Refactor `op/` to support both Go and Glojure function implementations behind the same `CustomFunction` interface
3. **Builtin namespace registration** — Go functions exposed to Glojure as callable namespaces
   - Start with 3 namespaces: `pudl.http` (generic HTTP), `pudl.exec` (subprocess), `pudl.core` (utilities)
4. **Upgrade CUE processor** — `internal/cue/processor.go` calls into the unified registry, supporting Glojure-backed functions alongside existing Go ones
5. **I/O-capable CUE functions** — CUE functions may perform I/O (HTTP requests, file reads) to fetch values. The processor handles:
   - Timeouts on function calls
   - Result caching (same function+args → same result, don't fetch twice)
   - Clear error reporting distinguishing eval-time from execution-time failures
6. **`pudl process` upgrade** — Works with both Go and Glojure functions seamlessly

**Reuses:** `op/` package (refactored), `internal/cue/processor.go` (upgraded), CUE evaluator.

**New packages:** `internal/glojure/` (runtime embedding, namespace registry).

### Phase 3b: Method Execution Pipeline (COMPLETE)

**Goal:** Method logic written in Glojure can be executed by the Go runtime, with lifecycle dispatch based on method kind.

**Status:** Complete. See `implog/2026_03_07_phase3b_method_execution.md` for details.

The executor package (`internal/executor/`) orchestrates method execution: loads `.clj` files, runs qualifications before actions, executes post-actions after. CLI commands `pudl method run` and `pudl method list` provide user access. Qualification terminology kept as-is in code (CUE schemas use "qualification"); "advice" used only as conceptual term.

Methods are `.clj` files that call Go-registered builtins via the Glojure runtime established in Phase 3a. Qualifications are renamed to **advice** to make the aspect-oriented nature explicit — they are cross-cutting concerns inserted at lifecycle cut-points.

1. **Method file convention** — `methods/<model-name>/<method-name>.clj` with `(defn run [args] ...)` entry point
2. **Method execution pipeline with lifecycle dispatch:**
   - Load definition → resolve args (including socket inputs from connected definitions) → bind to Glojure env
   - **Before action:** find all `advice` methods that declare `blocks: [<this-method>]`, execute them first. If any return `{passed: false}`, abort with the advice message.
   - Evaluate `.clj` file → call `(run args)`
   - Validate return value against CUE return schema (advice methods validate against `#AdviceResult`)
   - **After action:** run any `attribute` methods to compute derived values; run `codegen` methods to produce output transforms
   - Store result as immutable data artifact (via existing pudl storage)
   - Update output socket values on the definition (available to downstream definitions)
3. **`pudl method run <definition> <method> [--tag k=v]`** — Execute a method (advice runs automatically)
4. **`pudl method run --dry-run <definition> <method>`** — Run advice only, show what would execute
5. **`pudl method run --skip-advice <definition> <method>`** — Bypass advice checks (requires explicit flag)
6. **`pudl method list <definition>`** — List available methods grouped by kind

**Reuses:** Glojure runtime (Phase 3a), CUE evaluator (return schema validation), content-addressed storage, catalog (artifact indexing), metadata writer, definition graph (Phase 2, for socket resolution).

**New packages:** `internal/executor/` (lifecycle dispatch, advice runner, socket value propagation).

### Phase 4: Artifact Management — Unify Storage (COMPLETE)

**Goal:** Method outputs, imported data, and workflow results share one storage and query layer.

**Status:** Complete. See `implog/2026_03_07_phase4_artifact_management.md` for details.

Method outputs are stored as catalog entries alongside imported data using the same content-hashing, dedup, and SQLite catalog. New columns (`entry_type`, `definition`, `method`, `run_id`, `tags`) discriminate artifacts from imports. CLI commands `pudl data search` and `pudl data latest` provide artifact querying. `pudl list` defaults to imports only (backwards-compatible); `--artifacts` and `--all` flags added.

Pudl's existing catalog becomes the unified artifact backend. Method outputs are stored with the same content hashing, dedup, and provenance tracking as imported data — but with richer metadata (definition, method, run-id, tags).

1. **Extend `CatalogEntry`** with execution metadata — `definition`, `method`, `run_id`, `tags`
2. **Artifact path convention** — `.pudl/data/<definition>/<method>/<timestamp>-<hash>.json` with `latest` symlink
3. **Tag system** — Key-value tags on definitions propagate to artifacts; overridable with `--tag`
4. **`pudl data search`** — Search artifacts by definition, method, tag, time range
5. **`pudl data latest <definition> <method>`** — Show most recent artifact
6. **Adapt existing `pudl list`/`pudl show`** to display execution artifacts alongside imported data

**Reuses:** SQLite catalog, content hash dedup, metadata writer, lister/query system, export command.

**New:** Migration to add execution columns; tag storage and query; artifact path conventions.

### Phase 5: Vault System — Credential Management (COMPLETE)

**Goal:** Secrets referenced in definitions are resolved securely at execution time.

**Status:** Complete. See `implog/2026_03_07_phase5_vault_system.md` for details.

Vault references (`vault://path`) in definition socket bindings are resolved by the executor immediately before method execution. Resolved values never hit disk or artifacts. Two backends: environment variables (default, CI-friendly) and age-encrypted file (`~/.pudl/vaults/default.age`). Config key `vault_backend` selects backend. CLI commands `pudl vault get/set/list/rotate-key` manage secrets.

1. **Vault interface** — `Get(path) → (string, error)` with backend implementations
2. **Environment vault** — Reads from env vars (default; suitable for CI)
3. **File vault** — Encrypted JSON files in `.pudl/vaults/` using `age`
4. **Vault resolution in executor** — Walk args map, substitute `vault://` references before method execution
5. **`pudl vault set/get/list`** — CLI for managing secrets
6. **`pudl vault rotate-key`** — Re-encrypt file vault with new key

**Reuses:** Config system (vault backend selection), executor args resolution.

**New packages:** `internal/vault/` (interface, env backend, file backend, factory).

### Phase 6: Workflows — DAG Orchestration ✅

**Goal:** CUE files describe ordered graphs of method executions with automatic dependency resolution.

Workflows are DAGs where nodes are method invocations and edges are CUE field references between steps. Steps with no data dependency run concurrently.

1. ✅ **Workflow CUE file format** — Steps with `definition`, `method`, `inputs`, `condition`, `timeout`, `retries`
2. ✅ **DAG builder** — Extract step dependencies from CUE field references
3. ✅ **Topological sort + concurrent execution** — `errgroup` for parallel steps; configurable abort-on-failure
4. ✅ **Step input/output threading** — `steps.<name>.outputs.<field>` resolved from prior step artifacts via `sync.Map`
5. ✅ **Workflow run manifest** — `.pudl/data/.runs/<workflow>/<run-id>.json` recording outcomes, timing, artifact paths
6. ✅ **`pudl workflow run/list/show/validate/history`** — Full workflow CLI
7. ✅ **Acceptance test** — SSH-based workflow acceptance test with testcontainers (gated behind `//go:build acceptance`)

**Reuses:** CUE evaluator, method execution pipeline (Phase 3), artifact storage (Phase 4).

**New packages:** `internal/workflow/` (DAG builder, scheduler, runner, manifest writer).

### Phase 7: Drift Detection ✅

**Goal:** Compare declared infrastructure state against live state using JSON deep diff.

**Status:** Complete. See `implog/2026_03_07_phase7_drift_detection.md` for details.

Drift detection compares a definition's declared state (socket bindings) against live state (method output artifacts). JSON deep diff reports added/removed/changed fields with dot-notation paths. Drift reports stored in `.pudl/data/.drift/<definition>/<timestamp>.json`. Optional `--refresh` flag re-executes the method before comparing via the `StepExecutor` interface.

1. ✅ **`pudl drift check <definition>`** — Compare declared vs live state, show field-level diffs
2. ✅ **`pudl drift check --all`** — Drift check across all definitions
3. ✅ **`pudl drift report <definition>`** — Display last saved drift report without re-running
4. ✅ **JSON deep diff** — Recursive field comparison with numeric type coercion, dot-notation paths
5. ✅ **Report storage** — `.pudl/data/.drift/<definition>/<timestamp>.json` with save/list/get/getLatest

**Reuses:** Definition discoverer, model discoverer, artifact storage (last known state), StepExecutor interface (from workflow), executor (for refresh).

**New packages:** `internal/drift/` (comparator, checker, report store).

### Phase 8: Agent Integration & Skill Files

**Goal:** AI agents can discover models, write definitions and methods, compose workflows, and present artifacts for human review.

1. **Skill markdown files** — Bundled into binary, written to `.claude/skills/` on init
   - `pudl-core/SKILL.md` — CLI usage, repo layout
   - `pudl-definitions/SKILL.md` — Writing CUE definitions
   - `pudl-methods/SKILL.md` — Writing Glojure methods
   - `pudl-workflows/SKILL.md` — Composing workflow DAGs
   - `pudl-models/SKILL.md` — Defining extension models
2. **`pudl model search <query>`** — Keyword search across model schemas
3. **`pudl model scaffold <name>`** — Generate model CUE schema + method stubs
4. **Effect description pattern** — Methods return `{:pudl/effects [...]}` instead of executing directly; runtime handles execution with audit trail and `--dry-run` support
5. **Extension model discovery** — User-defined models in `extensions/models/` auto-discovered

**Reuses:** Everything — this phase is the capstone that ties the system together for agent use.

---

## Original Analytics Roadmap (Preserved)

The following items from the original roadmap remain relevant and can be pursued in parallel or integrated into the phases above where noted.

### Analytics: Analytical Layer

1. **`pudl diff`** — Compare two versions of the same resource (integrates with Phase 7: Drift)
2. **`pudl summary` / `pudl stats`** — Aggregate views
3. **Basic outlier detection** — Unusual field values across instances of a schema

### Analytics: Schema Intelligence

1. **Two-tier schema system** — Broad type recognition + policy compliance
2. **Schema drift detection** — Integrates with Phase 7
3. **Schema coverage reports** — Schema match distribution across data

### Analytics: Correlation & Cross-Source

1. **Cross-source correlation** — Link resources across providers
2. **Temporal tracking** — Same resource across imports (enabled by `resource_id` + `version`)

### Analytics: Advanced

1. **DuckDB/Parquet integration** — Analytical queries for large datasets
2. **Expert system components** — Common substructure detection
3. **Dashboard/reporting** — Visual infrastructure state

## Cut Candidates

Identified in project review but not yet addressed:

- `op/` + `internal/cue/processor.go` + `cmd/process.go` — CUE custom function processor (may be repurposed for CUE evaluation pipeline)
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

### Existing (Data Pipeline)

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

### New (Infrastructure Automation)

| Package | Path | Phase | Responsibility |
|---------|------|-------|----------------|
| `model` | `internal/model/` | 1 | Model discovery, method/socket extraction, lifecycle resolution |
| `definition` | `internal/definition/` | 2 | Definition loader, validator, socket wiring, dependency graph |
| `glojure` | `internal/glojure/` | 3a | Runtime embedding, namespace registry, CUE function bridge |
| `executor` | `internal/executor/` | 3b | Lifecycle dispatch, advice runner, socket value propagation |
| `artifact` | `internal/artifact/` | 4 | Artifact serialization, hashing, storage, dedup |
| `vault` | `internal/vault/` | 5 | Vault interface, env/file backends, resolution walker |
| `workflow` | `internal/workflow/` | 6 | DAG builder, scheduler, runner, manifest writer |
| `drift` | `internal/drift/` | 7 | State comparator, report generator |
