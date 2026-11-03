# GCP pack: seven more types through Cloud Asset Inventory (pudl-q4v.4, part 2)

**Date:** 2026-11-03

## Work done

`gcp-gke-clusters`, `gcp-run-services`, `gcp-pubsub-topics`,
`gcp-service-accounts`, `gcp-dns-zones`, `gcp-kms-keyrings` and
`gcp-kms-keys` now search Cloud Asset Inventory first and fall back to
gcloud, bringing the CAI types to 13 of 18.

### Parity, checked live before and after

Field-path comparison of raw output on one real project, then an end-to-end
run of pudl itself (CAI, then `_gcpAssetScope: "off"`): all 13 CAI types
store 0 new records the second time. The end-to-end run found one
difference the raw comparison had missed: CAI omits a service account's
`disabled: false`.

Shapes added (`gcp_assets.cue`):

- GKE clusters: drop node pools' `kubeletCertInfo` (gcloud only).
- Service accounts: drop `etag`; always state `disabled` (default false).
- DNS zones: drop every nested `kind` (three levels, `labels` untouched), set
  the zone's `kind`, lower-case `visibility`.
- Cloud Run services, Pub/Sub topics, KMS key rings and keys: unchanged.

The project now comes from `projects/<id>` in the parent's name, which also
covers KMS keys (parent: their key ring). CAI lists KMS resources in every
location, so the live search keeps `_gcpKMSLocations` like the gcloud
commands (fixtures are not filtered).

Stay on gcloud, with reasons in the README and design: compute instances,
Cloud SQL instances, buckets, project IAM policies (pudl-q4v.5), Pub/Sub
subscriptions (not compared).

## Public API (GCP pack)

- `_gcpAssetTypes` gains the seven types; `_gcpAssetInLocations`
- `_gcpAssetTransform._locations: *null | [...string]`
- `_gcpShapes` gains the seven shapes; `_gcpSameShape`;
  `_gcpNoKind0` to `_gcpNoKind2`

## Tests

- Synthetic CAI fixtures for the seven types; the gcloud fixtures gained a
  node-pool `kubeletCertInfo` and a `disabled: false`, the CAI ones omit
  them, drop `kind`s and upper-case visibility, as live.
- `TestGCPPackAssetSourcesStoreIdenticalRecords` covers all 13 types;
  mutations of the DNS and service-account shapes fail it.
- `TestGCPPackCloudAssetKMSKeepsConfiguredLocations` (fake gcloud): only
  global keys by default, both locations when configured.
- The fake gcloud now lists instances; the non-CAI assertion uses
  `gcp-instances`.

CUE note: `&&` does not short-circuit, so conditional lookups use the
list-select pattern `[if cond {x}, y][0]`, whose unselected element may
fail.
