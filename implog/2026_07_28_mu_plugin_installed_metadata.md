# Installed mu plugin metadata handoff

## Outcome

Completed the PUDL side of the catalog-installed plugin slice. PUDL now
consumes metadata from the installed mu bundle instead of requiring a copied
plugin source directory.

## Public API and behavior

- `mubridge.InstalledPluginBundleDir(name, digest)` resolves mu's stable
  extracted bundle path.
- `mubridge.LoadInstalledPluginPackage(projectRoot, name, digest)` reads the
  generated `mu-plugin.json` metadata, with `mu.lock` as the project fallback.
- `mubridge.SyncInstalledPluginSchemas` imports plugin-owned schemas into the
  append-only PUDL schema cache and returns the PUDL mappings.
- `pudl run` preflights digest-backed observers before invoking mu, synchronizes
  their `mu/...` schemas, validates their `pudl/...` mappings, and reports an
  exact `mu plugin install NAME[@VERSION]` command when the bundle is absent.
- Local `script` plugin metadata continues to use `mu.cue` and `pudl.cue`.

## mu handoff

The catalog installer now preserves `schemas` and `pudl_mappings` in
`mu.lock` and writes the same package metadata to `mu-plugin.json` inside the
extracted bundle. This keeps PUDL and mu independently installable while
making the installed package self-describing.

## Verification

- PUDL focused `internal/mubridge` and `cmd` tests pass.
- mu focused `internal/plugincatalog` and `cmd/mu` tests pass.
- Tests cover schema synchronization from an installed bundle and exact
  missing-plugin guidance.
- Disposable live smoke: installed `aws@0.1.0` from the published catalog,
  ran `pudl run --populate plugin:aws` with the fake AWS CLI, and observed one
  record classified as `pudl/aws.#Instance`; the synchronized `mu/aws@v1`
  files were present in PUDL's cache.
