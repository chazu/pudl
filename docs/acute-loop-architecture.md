# ACUTE Loop & Per-Repo Workspace Architecture

This document specifies the work needed to close the ACUTE convergence loop in pudl and add per-repo workspace support so BRICK patterns are portable across repositories.

## Context

pudl's outbound path works: drift detection finds differences, `export-actions` emits mu.json, mu converges. But the return path is broken -- mu's results (manifests, observe output) don't feed back into pudl state. And all pudl data lives in `~/.pudl/`, making definitions non-portable across repos.

This document covers six areas of work:

1. Observe result ingestion (close the feedback loop)
2. Manifest ingestion (record what mu did)
3. Convergence status tracking (queryable per-resource state)
4. BRICK-aware toolchain mapping (make BRICK load-bearing)
5. Per-repo workspace support (portable definitions)
6. Workspace-aware schema resolution (per-repo + global layering)

---

## 1. Observe Result Ingestion

### Problem

`pudl/mu.#ObserveResult` is defined as a CUE schema, but there is no command or logic that takes `mu observe --json` output and uses it as the "live state" side of drift detection. Currently drift compares definitions against the last generic import, not against what mu actually observed.

### Design

Add an `ingest-observe` command that:

- Accepts NDJSON from stdin or a file path (mu observe emits one JSON object per target)
- Validates each object against `pudl/mu.#ObserveResult`
- For each observe result:
  - Extracts the `target` field to identify the resource
  - Stores the observed state as a catalog entry with `entry_type: "observe"` and `origin: "mu-observe"`
  - Associates it with the corresponding `resource_id` by matching the target name to a definition
- The stored observe result becomes the "live" side for subsequent `drift check` calls

### Interface

```
mu observe --json | pudl ingest-observe
pudl ingest-observe --path observe-results.json
```

### Changes Required

| Package | Change |
|---------|--------|
| `cmd/` | New `ingest_observe.go` command |
| `internal/mubridge/` | New `ingest.go` with `IngestObserveResults()` function |
| `internal/database/` | Support `entry_type: "observe"` in queries and `GetLatestObserve(definitionName)` |
| `internal/drift/checker.go` | Prefer observe results over generic imports as "live state" source |

### Key Decisions

- Observe results are stored as catalog entries (not a separate table) -- reuses existing infrastructure
- `entry_type` field already exists with values "import" and "artifact"; we add "observe"
- Drift checker prefers observe results when available, falls back to latest import

---

## 2. Manifest Ingestion

### Problem

When mu finishes a build, it emits a manifest describing what actions ran, whether they succeeded, and what outputs they produced. pudl has a `pudl/mu.#Manifest` schema but no code to ingest manifests and update state.

### Design

Add an `ingest-manifest` command that:

- Accepts a manifest JSON file (from `mu build --emit-manifest`)
- Validates against `pudl/mu.#Manifest`
- For each action in the manifest:
  - Records the action result as a catalog entry with `entry_type: "manifest"`, `origin: "mu-build"`
  - Links to the definition via `definition` field
  - Stores `run_id`, exit code, cached status, and timestamp
- Updates the convergence status of affected resources (see section 3)

### Interface

```
mu build --emit-manifest | pudl ingest-manifest
pudl ingest-manifest --path manifest.json
```

### Changes Required

| Package | Change |
|---------|--------|
| `cmd/` | New `ingest_manifest.go` command |
| `internal/mubridge/` | New `manifest.go` with `IngestManifest()` function |
| `internal/database/` | Query helpers: `GetManifestsByRunID()`, `GetLatestManifest(definitionName)` |

### Key Decisions

- Manifests are catalog entries, same as observe results -- consistent model
- Each action becomes its own entry (not one entry per manifest) for granular tracking
- `run_id` groups entries from the same mu build invocation

---

## 3. Convergence Status Tracking

### Problem

After an ACUTE cycle, there's no way to ask pudl "what's the status of this resource?" The catalog tracks versions and content hashes, but not convergence state.

### Design

Add a `status` column to `catalog_entries` with possible values:

- `unknown` -- default for all existing and newly imported entries
- `clean` -- drift check found no differences
- `drifted` -- drift check found differences
- `converging` -- mu build is in progress (set when export-actions runs)
- `converged` -- manifest shows successful convergence
- `failed` -- manifest shows failed convergence

Status is updated by:

- `drift check` sets `clean` or `drifted` on the definition's latest observe/import entry
- `export-actions` sets `converging` on affected entries
- `ingest-manifest` sets `converged` or `failed` based on action exit codes

Add a `pudl status` command that shows per-definition convergence state:

```
$ pudl status
  api_server      converged   (last: 2026-03-24T10:15:00Z)
  monitoring      drifted     (3 differences)
  config_file     clean
```

### Changes Required

| Package | Change |
|---------|--------|
| `internal/database/` | Migration to add `status` column with default `"unknown"` |
| `internal/database/` | `UpdateStatus(resourceID, status)` method |
| `internal/drift/checker.go` | Update status after check |
| `cmd/export_actions.go` | Update status to `converging` on export |
| `internal/mubridge/manifest.go` | Update status on manifest ingest |
| `cmd/` | New `status.go` command |

### Key Decisions

- Status lives on catalog entries (not a separate table) -- keeps the model simple
- Status is denormalized onto the latest entry per resource_id for fast queries
- `pudl status` reads from catalog, grouping by definition name

---

## 4. BRICK-Aware Toolchain Mapping

### Problem

`export-actions` infers toolchains from schema name prefixes (`k8s.` -> k8s, `ec2.` -> aws). BRICK `#Target` has an explicit `toolchain` field. When definitions use BRICK targets, the toolchain should come from the BRICK metadata, not prefix heuristics.

### Design

Modify `ExportMuConfig` to:

1. Check if the definition's declared state contains a `toolchain` field (present in all `brick.#Target` instances)
2. If found, use it directly -- no prefix matching needed
3. If not found, fall back to the existing prefix heuristic

Also modify `DriftInput` to carry an optional `BrickToolchain` field populated by the command layer when the definition's schema ref matches `brick.#Target`.

### Changes Required

| Package | Change |
|---------|--------|
| `internal/mubridge/export.go` | Check `DriftInput.BrickToolchain` before prefix resolution |
| `internal/mubridge/export.go` | New `BrickToolchain` field on `DriftInput` |
| `cmd/export_actions.go` | Detect BRICK targets and populate `BrickToolchain` |

### Key Decisions

- BRICK toolchain takes absolute precedence over prefix heuristics
- Detection is simple: if `SchemaRef` contains `brick.#Target`, read the `toolchain` field from declared keys
- No new dependencies -- just a priority check in existing code

---

## 5. Per-Repo Workspace Support

### Problem

All pudl state lives in `~/.pudl/`. Definitions can't be shared via git. Two repos can't have independent BRICK targets. Cloning a repo doesn't give you its pudl context.

### Design

#### Workspace Discovery

pudl walks up from `cwd` looking for a `.pudl/workspace.cue` file. When found, that directory is the "workspace root." If not found, pudl uses `~/.pudl/` as before (global mode).

#### Per-Repo Layout

```
my-project/
  .pudl/
    workspace.cue          # workspace identity + config overrides
    schema/                # project-specific CUE schemas
      myapp.cue
    definitions/           # desired state (BRICK targets, definitions)
      api-server.cue
      monitoring.cue
  src/
  ...
```

#### workspace.cue

```cue
package workspace

// Workspace identity -- used as origin in catalog entries
name: "my-project"

// Optional: override toolchain mappings for this repo
toolchain_mappings: [
    {prefix: "monitoring", toolchain: "shell"},
]

// Optional: additional schema search paths
schema_paths: []
```

#### What Goes Where

| Data | Location | Rationale |
|------|----------|-----------|
| Definitions (desired state) | Per-repo `.pudl/definitions/` | Must travel with code, shared via git |
| Project-specific schemas | Per-repo `.pudl/schema/` | Same -- project-scoped types |
| Toolchain overrides | Per-repo `.pudl/workspace.cue` | Different repos may map differently |
| SQLite catalog | Global `~/.pudl/data/sqlite/catalog.db` | Machine-local runtime state |
| Base schema library | Global `~/.pudl/schema/` | Shared across repos |
| Bootstrap schemas | Embedded in binary | Always available |
| Drift reports | Global `~/.pudl/data/.drift/` | Ephemeral runtime state |
| Observe/manifest results | Global catalog (with workspace origin) | Runtime state, scoped by origin |
| Secrets/vault | Global `~/.pudl/vaults/` | Never per-repo |

#### Catalog Scoping

When running in a workspace, pudl sets `origin` on all catalog operations to the workspace name from `workspace.cue`. This enables:

- `pudl list` shows only entries from the current workspace by default
- `pudl list --all-workspaces` shows everything
- `pudl drift check` loads definitions from the workspace, not global
- `pudl export-actions` uses workspace toolchain mappings

#### Config Resolution Order

When pudl loads configuration:

1. Per-repo `.pudl/workspace.cue` (highest priority)
2. Global `~/.pudl/config.yaml` (defaults)
3. Hardcoded defaults (fallback)

For toolchain mappings specifically: workspace mappings are checked first, then global config mappings, then `DefaultMappings`.

### Changes Required

| Package | Change |
|---------|--------|
| `internal/workspace/` | **New package**: `Discover()`, `Load()`, `WorkspaceConfig` struct |
| `internal/config/` | Add `WorkspaceName`, `WorkspaceRoot` fields; merge logic |
| `internal/repo/init.go` | Generate `workspace.cue` with name derived from directory |
| `cmd/root.go` | Call workspace discovery on startup, inject into config |
| `internal/definition/discovery.go` | Accept multiple definition directories |
| `internal/database/` | Default `origin` filter to workspace name when in a workspace |
| `cmd/list.go` | Add `--all-workspaces` flag |

### Key Decisions

- One global catalog DB with origin-based scoping (not per-repo databases)
- `workspace.cue` is the marker file (like `.git/` for git)
- Workspace name must be unique -- it's the scoping key in the catalog
- `pudl repo init` already creates `.pudl/`; extend it to create `workspace.cue`

---

## 6. Workspace-Aware Schema Resolution

### Problem

Schema inference and validation currently look only at `~/.pudl/schema/`. With per-repo workspaces, project-specific schemas need to be found alongside global schemas.

### Design

Schema resolution checks directories in order:

1. Per-repo `.pudl/schema/` (project-specific schemas)
2. Global `~/.pudl/schema/` (shared schemas)
3. Embedded bootstrap schemas (`pudl/core`, `pudl/brick`, `pudl/mu`)

For CUE module resolution, the per-repo schema directory can import from the global schema directory using CUE's module system. The per-repo `cue.mod/module.cue` can declare a dependency on the global schemas module.

### Changes Required

| Package | Change |
|---------|--------|
| `internal/inference/` | Accept schema path list instead of single path |
| `internal/schema/` | Schema discovery across multiple directories |
| `internal/validator/` | Validate against merged schema set |
| `internal/definition/discovery.go` | Search per-repo then global definitions directories |
| `internal/importer/` | Pass merged schema paths to inference |

### Key Decisions

- Per-repo schemas shadow global schemas with the same name
- CUE module imports handle cross-directory dependencies
- Bootstrap schemas are always available regardless of workspace state

---

## Dependency Graph

```
                    +-----------------+
                    | 5. Per-Repo     |
                    |    Workspace    |
                    +--------+--------+
                             |
                    +--------v--------+
                    | 6. Schema       |
                    |    Resolution   |
                    +--------+--------+
                             |
              +--------------+--------------+
              |                             |
    +---------v---------+         +---------v---------+
    | 1. Observe        |         | 4. BRICK          |
    |    Ingestion      |         |    Toolchain      |
    +---------+---------+         +-------------------+
              |
    +---------v---------+
    | 2. Manifest       |
    |    Ingestion      |
    +---------+---------+
              |
    +---------v---------+
    | 3. Status         |
    |    Tracking       |
    +-------------------+
```

**Critical path:** 5 -> 6 -> 1 -> 2 -> 3 (workspace must exist before ingestion can be workspace-scoped)

**Independent:** 4 (BRICK toolchain mapping) can be done at any time

**Minimum viable loop:** 1 + 2 + 3 gives a closed ACUTE cycle using global `~/.pudl/` only. 5 + 6 make it portable.

---

## Estimated Scope

| Area | New Files | Modified Files | New Lines (est.) |
|------|-----------|----------------|------------------|
| 1. Observe ingestion | 2 | 2 | ~200 |
| 2. Manifest ingestion | 2 | 1 | ~200 |
| 3. Status tracking | 2 | 4 | ~250 |
| 4. BRICK toolchain | 0 | 2 | ~30 |
| 5. Per-repo workspace | 4 | 5 | ~350 |
| 6. Schema resolution | 0 | 5 | ~150 |
| **Total** | **10** | **19** | **~1,180** |
