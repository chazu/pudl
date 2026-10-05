# pudl-09o: Fix 'Failed to initialize enhanced importer' error

## Problem
Running `pudl import --path test/data/iam_roles.json` failed with:
```
Error: Failed to initialize enhanced importer
```

Root cause: `ensureBasicSchemas()` in `internal/importer/cue_schemas.go` checked for
`pudl/core/core.cue` in the schema repository but only returned an error if it was
missing. The `core.cue` bootstrap schema was added after the user had already run
`pudl init`, so existing installations were missing it.

## Fix
Changed `ensureBasicSchemas()` to auto-copy missing bootstrap schemas when the schema
repository is initialized (cue.mod exists) but individual bootstrap files are absent.
This makes the importer self-healing for cases where new bootstrap schemas are added
after initial setup.

## Files changed
- `internal/importer/cue_schemas.go` — `ensureBasicSchemas()` now calls
  `copyBootstrapSchemasTo()` to fill in missing bootstrap schemas before failing.

## Public API
No API changes. The fix is internal to the importer initialization path.
