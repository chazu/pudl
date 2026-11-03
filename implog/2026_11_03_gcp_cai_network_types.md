# GCP pack: network types through Cloud Asset Inventory (pudl-q4v.4, part 1)

**Date:** 2026-11-03

## Work done

The live collectors of six types now search Cloud Asset Inventory (CAI)
first and fall back to their per-project gcloud commands:
`gcp-networks`, `gcp-subnets`, `gcp-firewalls`, `gcp-routes`, `gcp-routers`,
`gcp-addresses`. By default one search per type covers every organization
the credentials can see, discovered at run time; nothing in the pack names
an organization, project or billing project.

### Parity checked on live data first

Each type's CAI copy was compared with gcloud's on one real project by field
path (never values):

- networks, subnets, firewalls, routes, routers, addresses: identical
  counts and values except `kind` (CAI omits it), `fingerprint` (subnets) and
  `labelFingerprint` (addresses), which can differ, and gcloud's derived
  `x_gcloud_*` fields (networks), which no schema reads.
- compute instances: **not** equivalent. CAI's instance metadata is a
  filtered view (on GKE nodes only a few keys; settings such as
  `disable-legacy-endpoints` missing) with lower-cased values, and Shielded
  VM certificates come back in standard rather than URL-safe base64.
  Decided with the user: instances stay on gcloud.

### One shape for both sources

`_gcpComputeShape` drops `kind`, `fingerprint`, `labelFingerprint` and
`x_gcloud_*` and sets the type's `kind` (the schemas match on it). The CAI
transform unwraps `versionedResources[0].resource`, takes the project from
`parentFullResourceName`, drops other asset types, and applies the shape; the
gcloud runs apply the same shape. Switching source stores no new versions.

## Public API (GCP pack, `examples/gcp/schema/models/`)

Settings (set outside the pack, e.g. `.pudl/schema/models/gcp_settings.cue`):

- `_gcpAssetScope: *"organization" | "projects" | "off"`
- `_gcpBillingProject: string | *""` (`--billing-project`; empty uses gcloud's
  `billing/quota_project`)

Machinery (`gcp_assets.cue`): `_gcpAssetTypes`, `_gcpAssetUnavailable`
(fallback patterns), `_gcpOrgList`, `_gcpAssetSearch`, `_gcpAssetTransform`,
`_gcpGcloudTransform`, `_gcpShapes`, `_gcpComputeShape`. In `gcp_pack.cue`,
`_#GCPCollector` now takes `_gcloud` and `_cai` sources and chooses CAI with
a fallback when `_useCAI`; `_gcpShapedRuns` adds the gcloud transform.

Fixtures: `populators/gcp/cai/<type>.json` (synthetic CAI search results,
one with a non-matching asset type); the gcloud fixtures gained a
fingerprint, a labelFingerprint and `x_gcloud_*` fields so the shape is
exercised.

CUE lessons recorded in comments: helpers are hidden fields, not
definitions (definitions close the structs they build); `{_t: _t}` and
`{in: in.x}` refer to themselves, so `let` aliases are used; a transform's
`out` must be declared outright, so the type check selects
`[if wrong-type {null}, shaped][0]`.

## Tests

- `cmd/gcp_pack_assets_test.go`: the six fixture collectors read CAI, then
  with `_gcpAssetScope: "off"` gcloud; the second run stores nothing new.
  Fails if the shape keeps fingerprints.
- `cmd/gcp_pack_live_test.go`: the fake gcloud lists an organization and
  fails CAI search with SERVICE_DISABLED by default (so the existing tests now
  run through the fallback), or answers it with `GCLOUD_CAI=1`. New tests:
  org-wide search, projects mode with a billing project, "off" storing
  identical records; fallback reported with its trigger; non-CAI types never
  search.
- `TestGCPContentPack`: unchanged expectations; fixture-dropping now edits
  the CAI fixture too.

## Remaining in pudl-q4v.4

GKE clusters, SQL instances, buckets, Cloud Run services, DNS zones, Pub/Sub
topics and subscriptions, KMS key rings and keys, service accounts and IAM
policies: each needs the same live parity check before it moves (buckets
also need a shape for the JSON API's camelCase fields).
