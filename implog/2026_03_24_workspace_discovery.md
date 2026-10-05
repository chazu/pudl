# B2.1 — Workspace Discovery and Loading

**Date:** 2026-03-24
**Bead:** B2.1

## Summary

Created the `internal/workspace/` package implementing per-repo workspace discovery and loading. This package walks up from the current working directory to find `.pudl/workspace.cue`, parses it with the CUE Go API, and provides a resolved context for commands.

## Public API

### `workspace.go`

- `type Workspace struct` — discovered per-repo workspace with Root, PudlDir, Name, SchemaPath, DefinitionsPath, ToolchainMappings
- `type ToolchainOverride struct` — maps a schema prefix to a toolchain name
- `func Discover(startDir string) (*Workspace, error)` — walks up from startDir looking for `.pudl/workspace.cue`; returns nil if no workspace found

### `context.go`

- `type Context struct` — resolved workspace state with Workspace, GlobalPudlDir, EffectiveOrigin, SchemaSearchPaths, DefinitionSearchPaths
- `func NewContext() (*Context, error)` — discovers workspace from cwd and builds resolved context with correct search path ordering (per-repo before global)

## Tests

9 tests in `workspace_test.go` and `context_test.go`:
- TestDiscover_Found, TestDiscover_NotFound, TestDiscover_WalksUp, TestDiscover_StopsAtRoot
- TestLoad_MinimalWorkspace, TestLoad_FullWorkspace, TestLoad_InvalidCUE
- TestNewContext_WithWorkspace, TestNewContext_GlobalOnly
