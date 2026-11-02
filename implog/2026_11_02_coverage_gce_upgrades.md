# Sparse graphs explain themselves; GCE instances; pack upgrades

Reported: `pudl ui` on a workspace with many imported GCP resources showed
little. Likely causes, all addressed:

1. Records with no resource identity never reach the graph, and the GCP pack
   had no Compute Engine instance type, so imported `gcloud compute instances
   list` output landed in a generic type without identity.
2. A pack installed before graph support has no `_pudl.graph` blocks or
   `graph_edge` rules, and `pudl example install` could not upgrade it
   (pudl-oti).
3. Nothing told the user any of this.

## Coverage

- `database.CatalogDB.SchemaCoverageCounts()`: entries and unidentified
  entries per schema, excluding collection wrappers.
- `graphexport.Meta.Coverage` (`Coverage`, `HiddenSchema`): entries,
  resources, hidden schemas with reason `no-identity` or `internal`, and the
  number of loaded edge rules.
- Web: a `CoveragePanel` in the sidebar ("N resources shown from M catalog
  entries", expandable per schema with what to do), a notice when no edge
  rules are loaded, and a hint when the current view has none of the
  resources.

## GCP Compute Engine instances

- `gcp.#ComputeInstance` (`examples/gcp/schema/gcp/compute.cue`):
  `kind: "compute#instance"` claims records, and identity is `selfLink`. Facts
  `gcp_instance`, `gcp_instance_nic` (per interface: network, subnetwork, IP,
  external IP present), `gcp_instance_sa` and `gcp_instance_tag`. Collector
  `gcp-instances` / `gcp-fixture-instances`, with a 3-VM fixture.
- Firewall facts gain `targeted`, `sa_targeted` and `disabled`, plus
  `gcp_firewall_target_tag`.
- Graph rules: `in_subnet`; `runs_as` (to the service account, which is not
  collected, so it is a reference); `applies_to` for enabled untargeted rules
  (every instance in the network) and tag-targeted rules (a matching tag in
  the network, with a `matched_tag` property). Rules that target service
  accounts are not matched.
- The exporter derives a region from a zone when the type names none
  (`us-central1-a` → `us-central1`, `us-east-1a` → `us-east-1`).
- Tests: fixture inference by plain `pudl import` (no `--schema`) picks the
  new type; graph export asserts placement, region, service account, tag and
  network matching, no tag match for untagged VMs, and none for disabled
  rules.

## Pack upgrades (pudl-oti)

- `internal/examples`: `Install(root, name, Options{Overwrite})` returns a
  `Result` (added, upgraded, replaced, unchanged, backups). The manifest
  `<workspace>/.examples/<name>.json` records the digest of every file
  written. A differing file whose digest matches the manifest is upgraded. An
  edited file, or one from a manifest-less install by an older pudl, is a
  `ConflictError` that lists every such file and suggests `--overwrite`.
  `--overwrite` keeps each old file as `<path>.bak[.N]`, which is not `.cue`,
  so CUE never loads it.
- Sources moved to `sources.go`, the manifest to `manifest.go`.
- Tests: manifest and idempotence, upgrade of an untouched file, edited and
  legacy files as conflicts with nothing written, and overwrite with
  successive backups.
