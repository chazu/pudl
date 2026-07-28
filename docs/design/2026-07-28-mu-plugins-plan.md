# Plan: official mu plugins and PUDL integration

**Date:** 2026-07-28
**Scope:** mu, mu-plugins, and PUDL
**Status:** §1 contract hardening complete; §2 AWS schema/mapping slice complete;
initial public `chazu/mu-plugins` migration pushed; catalog/release and mu
source-package commands complete; §5 installed-bundle handoff complete

Initial implementation evidence (2026-07-28):

- `discover` is now static in the AWS observer; AWS CLI validation runs at
  `observe` time.
- The Go SDK treats `Plan` as optional, derives `plan` only when a handler is
  present, and returns a truthful unsupported-capability response otherwise.
  The observer-only `host` plugin and secret-provider shortcut now use that
  contract.
- Discover metadata now supports both legacy `output_schema` and plural
  `output_schemas` entries with `resource_type` mappings.
- The plugin process now rejects malformed explicit capabilities and invalid
  schema references while preserving legacy empty-capability compatibility.
- Added a record-observer validator and wire-level fixture coverage for
  `current.records[]` plus non-empty `_schema` discriminators. AWS has an
  opt-in fake-CLI fixture that proves observe behavior without contacting AWS.
- Updated mu's protocol, SDK, plugin-author, and architecture docs.
- Targeted tests, `go test ./...`, `go build ./...`, `go vet ./...`, AWS
  discover/fixture runs, and bundled discovery scenarios for the shipped
  plugins pass.

Broader per-plugin fake-dependency fixtures remain migration work; schema
declarations and PUDL mappings are now the next architectural slice.

§2 implementation evidence (2026-07-28):

- AWS declares `mu/aws@v1` in its manifest, bundles the plugin-owned wire
  schemas, and advertises one output schema per emitted resource type.
- AWS bundles `pudl.cue` mappings to PUDL-owned `pudl/aws` semantic schemas;
  the plugin schema does not enter the `pudl/...` namespace.
- PUDL can load the file-level package contract, materialize plugin schemas in
  its append-only `muschemas` cache, reject reserved namespaces, validate
  mappings against the active inheritance graph, and route records using an
  explicit mapping before falling back to the legacy naming convention.
- `pudl mu ingest-observe --plugin-dir ...` and local-script `pudl run` use this
  path. Remote/digest bundle path handoff is implemented by the mu
  package-manager metadata seam described in §5.

§3 implementation evidence (2026-07-28):

- Created public [`chazu/mu-plugins`](https://github.com/chazu/mu-plugins) and
  cloned it at `/Users/chazu/dev/go/mu-plugins`.
- Migrated the manifest-selected canonical implementation for all 18 packages:
  Babashka entrypoints plus the Go `envsecret` source provider, with guides,
  host support files, AWS schemas/mappings, and the deterministic AWS fixture.
- Pushed commit `98719f7` to `main`. The original `mu/plugins` tree remains in
  place during the compatibility window; duplicate unselected Go ports were
  not copied into the catalog.
- Added `catalog.source.json`, deterministic `catalog.json` generation, and
  SHA-256-addressed source archives for all 18 packages.
- Added catalog reproducibility checks, plugin contract smoke tests, and GitHub
  CI/release workflows. GitHub-backed installation in mu remains the next
  slice.

The recommendation is to make `chazu/mu-plugins` the official source/catalog,
while keeping mu's existing CAS as the installed runtime cache. GitHub becomes
the easy distribution channel; OCI remains optional for teams that want remote
artifact caching.

The public `chazu/mu-plugins` repository now exists. Its initial migration
contains all 18 current plugins:

- Observers/convergence: `aws`, `host`, `k8s`, `terraform`, `file`,
  `remote-file`, `remote-exec`
- Build/toolchain: `go`, `zig`, `docker`, `lint`, `scratch`, `cowsay`
- Secrets/lifecycle: `pass`, `sops`, `envsecret`, `keypair-gen`, `void`

## 1. Harden the contract first

Before moving code:

1. Make `discover` pure: no credentials, network calls, or required external
   binaries. The AWS CLI check must move from `discover` to `observe`.
2. Make capabilities truthful. The Go SDK currently forces every plugin to
   advertise `plan`; observer-only plugins should not need a fake no-op plan
   handler.
3. Define the stable observe shape:
   - `current.records`
   - one stable `_schema` resource type per record
   - deterministic error/partial-result behavior
4. Extend output-schema metadata from one schema to a list of resource-type
   mappings, since AWS and host emit multiple record types.
5. Add a conformance suite that runs against every plugin and checks discovery,
   capabilities, config schemas, fixtures, and external-dependency behavior.

That gives us a real contract before the new repository becomes an ecosystem.

## 2. Schema ownership

There should be two deliberately different schema layers:

| Concern | Location | Meaning |
|---|---|---|
| Plugin wire/output schema | `mu-plugins/plugins/aws/schemas/mu/aws/` | The JSON shape the plugin emits |
| PUDL catalog schema | PUDL's `pudl/aws` package | Identity fields, tracked fields, inference, Datalog/catalog semantics |
| Mapping between them | Plugin package metadata, e.g. `plugins/aws/pudl.cue` | `aws.ec2.instance → pudl/aws.#Instance` |

Plugin-owned output schemas should live with the plugin and travel in its
bundle. `pudl/aws` stays in PUDL: it contains PUDL-specific catalog semantics
and must remain available even when the plugin is not installed.

This matches the existing mu design: mu already supports vendored schemas in
plugin bundles and has a schema cache. Extend that path rather than creating a
separate `mu-schemas` repository.

The plugin repo must not overwrite PUDL's `pudl/...` namespace.

## 3. Create and migrate `mu-plugins`

Proposed repository structure:

```text
mu-plugins/
  mu.cue
  cue.mod/module.cue
  go.mod
  catalog.json              # generated
  plugins/
    aws/
      mu.cue
      plugin.bb
      GUIDE.md
      schemas/mu/aws/
      pudl.cue
    k8s/
      ...
  scripts/
  README.md
  .github/workflows/
```

Before copying, choose one canonical implementation per plugin. Several
plugins currently contain both BB and Go versions, while the manifest selects
only one. The new repo should not ship two competing implementations without a
reason.

The repository CI should run:

- `mu validate`
- per-plugin discovery tests
- fake AWS/kubectl/SSH fixtures
- PUDL projection/schema compatibility tests
- build/package tests for every plugin

## 4. Add GitHub-backed package management to mu

§4 implementation evidence (2026-07-28):

- `mu-plugins/cmd/catalog` validates package manifests against the migrated
  directories, creates deterministic source archives, and emits the release
  catalog with asset URLs and SHA-256 hashes.
- `catalog.json` is checked in as the reproducible development catalog;
  `catalog.source.json` is its human-maintained source of package metadata.
- `scripts/check-catalog.sh` verifies regeneration and asset completeness;
  `scripts/test-plugins.sh` runs the shipped discovery/fixture contract suite.
- `.github/workflows/ci.yml` runs Go, catalog, and plugin-contract checks;
  `.github/workflows/release.yml` generates release assets and publishes them
  with `gh release create` on `catalog-v*` tags.
- Published `catalog-v0.1.0`, then `catalog-v0.1.1` after making the source-only
  `envsecret` package self-contained; the latter is the current `latest`
  catalog release.
- Implemented `mu plugin search`, `install`, `update`, and `lock`. Install
  downloads and verifies the catalog asset, safely extracts it, runs declared
  source builds, bundles through the existing CAS resolver, updates `mu.cue`,
  and records catalog/asset/bundle pins in `mu.lock`.
- The live path was verified by installing `aws@0.1.0` and the Go source-only
  `envsecret@0.1.0` into an isolated project using the published release.

Add a generated release catalog and commands such as:

```text
mu plugin search
mu plugin install aws
mu plugin install aws@0.1.0
mu plugin update aws
mu plugin status
```

Recommended install flow:

1. Fetch a catalog from the official GitHub release.
2. Resolve the plugin name/version to an immutable release asset.
3. Verify the asset SHA-256.
4. Extract the plugin source.
5. Build/bundle it through the existing local-plugin resolver.
6. Store the resulting bundle in `~/.mu/plugins` / CAS.
7. Write the content digest into the project's `mu.cue`.
8. Record source revision, plugin version, asset hash, and bundle digest in
   `mu.lock`.

The catalog should contain entries shaped like:

```json
{
  "name": "aws",
  "version": "0.1.0",
  "asset_url": "...",
  "sha256": "...",
  "path": "plugins/aws",
  "entrypoint": "plugin.bb",
  "toolchain": "bb",
  "requirements": ["aws-cli >= 2"],
  "schemas": [],
  "pudl_mappings": []
}
```

Use GitHub release assets rather than mutable `main` files for installation. A
source-first package is enough for v1; prebuilt platform-specific binaries can
come later.

Keep these existing paths:

- local `script:` plugins for development
- digest-based plugins for reproducible projects
- OCI push/pull for users who want it

## 5. Wire it into PUDL

PUDL should:

1. Detect when a requested plugin is absent and report the exact install
   command.
2. Read the installed plugin's schema declarations.
3. Materialize plugin-owned schemas into its append-only `muschemas` cache.
4. Validate the plugin's declared PUDL mappings against built-in schemas.
5. Continue using `pudl/aws`, `pudl/linux`, and `pudl/k8s` for catalog
   classification.

The first end-to-end acceptance path should be:

```text
mu plugin install k8s
pudl model new cluster --populate plugin:k8s
pudl run cluster
pudl query ...
```

This should require no hand-authored plugin registration, no copied schema
files, and produce no dangling schema references.

## 6. Task order

1. `mu`: protocol/SDK hardening.
2. `mu` + PUDL: conformance fixtures and explicit schema mappings.
3. Create `chazu/mu-plugins`.
4. Canonicalize and migrate all 18 plugins.
5. Add generated catalog and GitHub release workflow.
6. Add `mu plugin install/update/lock`.
7. Add PUDL schema synchronization and missing-plugin guidance.
8. Update mu docs and PUDL scaffolding guidance.
9. Release one compatibility version.
10. Remove plugin implementations from the mu core repository.

## 7. Installed-bundle handoff evidence

- `mu.lock` now preserves catalog `schemas` and `pudl_mappings` for each
  selected package.
- `mu plugin install` writes `mu-plugin.json` beside the extracted bundle in
  `~/.mu/plugins/<name>/bundle-<digest>/`, allowing PUDL ad-hoc runs to work
  even when no project lockfile is present.
- PUDL resolves digest-backed models before invoking mu, reads the installed
  metadata, syncs plugin-owned schemas into `muschemas`, and validates the
  mappings against its loaded `pudl/...` schema graph.
- Missing bundles and unavailable package metadata produce an actionable
  `mu plugin install NAME[@VERSION]` command.
- Focused tests cover lock metadata round-tripping, installed schema sync,
  short/full digest compatibility, and missing-plugin guidance. A disposable
  live run installed `aws@0.1.0`, used the fake AWS CLI, synchronized
  `mu/aws@v1`, and classified one record as `pudl/aws.#Instance`.

## Recommendation

- `mu-plugins` owns plugin implementations and plugin-owned `mu/...` output
  schemas.
- PUDL owns semantic `pudl/...` catalog schemas.
- GitHub releases provide source packages and the catalog.
- mu's CAS remains the installed runtime mechanism.
- No separate schema repository yet.

The main implementation choice to pin is whether v1 installs source packages or
prebuilt binaries. Source packages are recommended first: the current plugin
set is mostly Babashka and mu already knows how to bundle local plugin
directories; prebuilt artifacts can follow once the contract is stable.
