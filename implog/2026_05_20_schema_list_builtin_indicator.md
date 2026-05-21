# Schema List Built-In Indicator

## Summary
Added `[built-in]` indicator to `pudl schema list` output showing which packages are shipped with pudl vs user-defined.

## Changes
- `internal/importer/cue_schemas.go`: Added `BootstrapPackages()` function that walks the embedded bootstrap FS to return the set of built-in package paths.
- `internal/schema/manager.go`: Added `BuiltIn` field to `SchemaInfo`, `SetBuiltInPackages()`/`isBuiltIn()` methods on `Manager`. Both `ListSchemas` and `GetSchemasInPackage` populate the field.
- `cmd/schema_list.go`: Wires up `importer.BootstrapPackages()` into the manager. Displays `[built-in]` tag on package headers in both all-packages and single-package views.

## Public API
- `importer.BootstrapPackages() map[string]bool` -- returns set of built-in package paths
- `SchemaInfo.BuiltIn bool` -- indicates whether schema belongs to a built-in package
- `Manager.SetBuiltInPackages(map[string]bool)` -- configures built-in package set
