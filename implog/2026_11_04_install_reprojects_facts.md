# Pack and example installs reproject stored facts (pudl-qaw)

## Problem

Upgrading the GCP pack changed types' `_pudl.facts` specs, but records already
in the catalog kept the facts the old specs projected until
`pudl doctor --fix --only projections` ran. Graph edges such as
`allows_ingress_from` were missing in the meantime.

## Root cause

`pudl import`, `check`, `delete`, `snapshot prune` and the doctor repairs end
with `syncProjectionsQuietly` (`cmd/projection_sync.go`), which reconciles
projected facts with the catalog and the current specs. The install commands
change schemas (they write the pack's CUE files) but never called it.

## What changed

- `cmd/fileset_install.go`: `installFileset` (shared by `pudl pack install`
  and `pudl example install`) calls `syncProjectionsAfterInstall` after a
  successful install. It syncs (via `syncProjectionsAfterWrite`, which opens
  the active catalog and runs `syncProjectionsQuietly`) only when the install
  added, upgraded or replaced files and the workspace already has a catalog
  (`data/sqlite/catalog.db`); a fresh workspace's catalog is not created.
  Output stays on stderr, so `--json` stdout is unchanged; a failed sync warns
  and names the doctor command, never failing the install.
- `pudl pack install --help`, `docs/cli/notes/{pack,example}_install.md`,
  `docs/projection.md`; regenerated `docs/cli-reference.md` and
  `cmd/testdata/help.golden.json`.

## Not done

The issue's second half — records pinned to a schema the pack replaced (e.g.
a workspace `gcp.#Instance` vs the pack's `gcp.#ComputeInstance`) still need
`pudl doctor --fix --only inference --schema ... --include-explicit`; install
does not detect two schemas claiming one `resource_type`.

## Public API

No new exported API. Behaviour: `pudl pack install` and
`pudl example install` reproject existing records' facts when they change
files.

## Tests

`cmd/pack_projection_test.go`:
- `TestPackUpgradeReprojectsStoredRecords`: firewalls checked under an
  older spec (`gcp_firewall_network_v0`); `pack install gcp --overwrite`
  reports `reprojected` and `query gcp_firewall_network` returns them with no
  doctor run. Fails with the sync call commented out.
- `TestPackInstallDoesNotCreateACatalog`.
