# Remove stale bootstrap definition examples (model/examples rot)

**Date:** 2026-06-15

## Summary

Deleted the three bootstrap definition example files that imported the removed
`pudl/model/examples` package:

- `internal/importer/bootstrap/definitions/http_def.cue`
- `internal/importer/bootstrap/definitions/simple_def.cue`
- `internal/importer/bootstrap/definitions/wired_defs.cue`

These were examples of the model / socket-wiring execution feature that was
extracted to **mu**. The `pudl/model/examples` package no longer exists, so each
file's `import "pudl.schemas/pudl/model/examples"` was unresolved.

## Why it mattered (latent bug)

`load.Instances("./...")` loads the whole schema module as a unit. One package
with an unresolved import fails the entire load: `CUEModuleLoader.LoadAllModules`
returns an error, and `inference.loadSchemasFromPaths` swallows it (`continue` on
error, `internal/inference/inference.go:76`), leaving **zero schemas loaded**.
Net effect in a real `~/.pudl/schema` populated from bootstrap: every import
silently fell through to the `pudl/core.#Item` catchall, and `pudl doctor`'s
identity-consistency check saw an empty schema set. Removing the broken files lets
the bootstrap schema module load cleanly.

This surfaced while adding the git schema family: `TestGitFamily*` had to drop the
`definitions/` tree to load the bootstrap; that workaround is now removed.

## Verification

- No Go code references the deleted files or the example models
  (`#HTTPEndpointModel`/`#SimpleModel`/`#VPCModel`/`#EC2InstanceModel`);
  `internal/definition/definition_test.go` uses its own inline fixtures.
- The `definitions/` dir held only these three files, so `git rm` removed the dir;
  `pudl init` still creates an empty `definitions/` dir in the target schema repo
  (`internal/init/init.go`), unaffected.
- `internal/importer/git_schema_test.go` now loads the bootstrap via
  `CopyBootstrapSchemas` with no `definitions/` workaround and passes.
- Full suite green (`CGO_ENABLED=0 go test ./...`).

## Files

- Deleted the three `.cue` files above (the `bootstrap/definitions/` dir is now gone).
- `internal/importer/git_schema_test.go` — removed the definitions-drop workaround.
