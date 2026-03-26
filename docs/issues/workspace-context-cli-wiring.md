# Wire workspace context into CLI for per-repo schemas and definitions

**Priority:** High
**Status:** Proposed
**Date:** 2026-03-25

## Summary

The `WorkspaceContext` infrastructure exists in code and tests (`internal/workspace/context_test.go`, `internal/definition/discovery.go:NewMultiDiscoverer`) but the CLI commands don't use it. All commands currently use only `cfg.SchemaPath` (the global `~/.pudl/schema/` path), ignoring per-repo `.pudl/` directories.

This means definitions and schemas placed in a project's `.pudl/definitions/` and `.pudl/schema/` are not discovered by `pudl definition list`, `pudl definition validate`, `pudl drift check`, `pudl export-actions`, or `pudl schema list`.

## What Exists

**Working infrastructure:**
- `workspace.Workspace` struct with `SchemaPath` and `DefinitionsPath` fields
- `workspace.buildContext()` produces `SchemaSearchPaths` and `DefinitionSearchPaths` (per-repo first, global second)
- `definition.NewMultiDiscoverer(schemaPaths)` searches multiple paths in order
- Per-repo shadowing: per-repo schemas shadow global schemas with the same name
- Tests: `TestMultiDiscoverer_PerRepoFirst`, `TestMultiDiscoverer_MergesBoth`, `TestInferrer_Shadowing`, `TestInferrer_FallbackToGlobal`

**Not wired up:**
- `repo.go` uses `definition.NewDiscoverer(cfg.SchemaPath)` (single global path)
- `export_actions.go` uses `definition.NewDiscoverer(cfg.SchemaPath)`
- `validate.go`, `schema.go`, etc. all use single path
- No command detects `.pudl/` in the current directory or walks up to find it

## Expected Directory Structure

```
my-project/
├── .pudl/
│   ├── workspace.cue       # Repo name and config
│   ├── definitions/        # Project-specific definitions (searched first)
│   │   ├── lint.cue        # e.g. BRICK targets for this project's linting
│   │   └── build.cue       # e.g. BRICK targets for this project's build
│   └── schema/             # Project-specific schemas (searched first)
│       └── custom.cue      # e.g. project-specific CUE types
│
~/.pudl/
├── schema/
│   ├── pudl/               # Base schemas (brick, core, mu, infra, etc.)
│   ├── definitions/        # Global definitions (searched second)
│   └── examples/           # Example schemas
└── data/                   # Catalog database
```

## Required Changes

### 1. Workspace discovery in every command

Every command that reads schemas or definitions needs to:
1. Walk up from cwd to find `.pudl/` directory (same as `FindProjectRoot` in mu)
2. Build a `WorkspaceContext` with per-repo + global search paths
3. Use `NewMultiDiscoverer(ctx.DefinitionSearchPaths)` instead of `NewDiscoverer(cfg.SchemaPath)`
4. Use `ctx.SchemaSearchPaths` for schema operations

**Commands to update:**
- `definition list` — search both paths
- `definition show` — search both paths
- `definition validate` — validate against both schema paths, CUE module resolution must work for both
- `definition graph` — discover from both paths
- `drift check` — discover definitions from both paths
- `export-actions` — discover definitions from both paths
- `schema list` — list schemas from both paths (indicate which are local vs global)
- `schema show` — search both paths
- `schema validate` — validate both paths
- `repo validate` — validate per-repo schemas and definitions
- `import` — when inferring schemas, search both paths

### 2. CUE module resolution for per-repo definitions

Per-repo definitions in `.pudl/definitions/` need to import schemas from the global `~/.pudl/schema/` CUE module. Options:
- **Symlink:** `.pudl/schema/` symlinks to `~/.pudl/schema/` (simple but fragile)
- **CUE module dependency:** `.pudl/` has its own `cue.mod/` that depends on the global module
- **Overlay:** When loading CUE, overlay the per-repo definitions onto the global schema module's filesystem

The overlay approach is probably best — it's what CUE's loader supports natively. The per-repo definitions would be evaluated as if they were part of the global schema module, with per-repo schemas taking precedence.

### 3. Origin tracking

When listing definitions, indicate where each one comes from:
```
$ pudl definition list
Available Definitions:

  lint_go_vet        brick.#Target     (repo: mu)
  lint_gofmt         brick.#Target     (repo: mu)
  some_global_def    infra.#Account    (global)
```

The `EffectiveOrigin` field from `WorkspaceContext` provides the repo name.

### 4. Workspace configuration

The `.pudl/` directory should optionally contain a `workspace.cue`:
```cue
name: "mu"
// Future: scoping rules, schema overrides, etc.
```

### 5. Fine-grained control

Users need control over:
- **Which global schemas are visible** to per-repo definitions (e.g. only import `pudl/brick`, not `pudl/infra`)
- **Definition scope** — a definition in `.pudl/definitions/` should only appear when working in that repo
- **Schema shadowing** — per-repo schemas shadow global ones by package path (already implemented in tests)
- **Export scope** — `pudl export-actions --all` from a repo should only export that repo's definitions, not global ones
- **`--scope` flag** — optionally filter by origin: `pudl definition list --scope=repo` vs `--scope=global` vs `--scope=all` (default when in a repo: repo-only; default outside a repo: global-only)

### 6. `pudl repo init` improvements

Currently `repo init` only creates an empty `.pudl/` dir and installs Claude skills. It should also:
- Create `.pudl/definitions/` and `.pudl/schema/` subdirectories
- Create `.pudl/workspace.cue` with the repo name (inferred from directory or git remote)
- Optionally create a `.pudl/cue.mod/` for per-repo CUE module resolution
- Print guidance on what goes where (global vs local)

### 7. Git integration

Per-repo definitions should be committed to git (they're project-specific desired state). Update `.pudl/` gitignore handling:
- `.pudl/definitions/` — committed (desired state)
- `.pudl/schema/` — committed (project-specific types)
- `.pudl/workspace.cue` — committed (project config)
- `.pudl/data/` — NOT committed (local cache, if it ever exists per-repo)

## Documentation Updates

- [ ] Update `docs/concepts.md` to explain global vs per-repo scope
- [ ] Update `docs/mu-integration.md` to show per-repo workflow
- [ ] Add a `docs/workspace.md` explaining the `.pudl/` directory structure, what goes where, and scoping rules
- [ ] Update the pudl Claude skill (`.claude/skills/`) to be aware of per-repo definitions and guide users to put project-specific definitions in `.pudl/definitions/`
- [ ] Update `pudl repo init` help text

## Motivation

The mu project has BRICK definitions for its lint and build targets in `.pudl/definitions/`. These are project-specific — they shouldn't appear when working in other repos. Currently they had to be placed in the global `~/.pudl/schema/definitions/` which pollutes the global namespace.

This pattern will be common: every project using mu+pudl will have its own BRICK targets that should be scoped to that project's `.pudl/` directory.
