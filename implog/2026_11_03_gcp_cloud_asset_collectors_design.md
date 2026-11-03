# GCP collectors on Cloud Asset Inventory: access research and design

**Date:** 2026-11-03

## Work done

- Checked what a regular (non-admin) GCP account can do with Cloud Asset
  Inventory (CAI) in a real organization. It can search resources org-wide
  and search IAM policies in some projects, but list full assets in only a
  few.
- Every CAI call is billed to a project that needs the API enabled and
  `serviceusage.services.use` for the caller; one was set up.
- Verified that search with `--read-mask=versionedResources` returns the full
  v1 API object using only the search permission.
- Ran an org-wide census of asset types reading only `assetType`: far more types than the pack covers. Found two API limits: the search query cannot filter on
  asset type, and every `--asset-types` pattern must match a supported type.
- Wrote the design: `docs/design/2026-11-03-gcp-cloud-asset-collectors.md`.

## Public API

None. No code changed. The design proposes these, all unimplemented:

- `#CommandObserve.fallback: {when, observe}`
- `#CommandRun.transform: {in, out}`
- a `sensitive_fields` entry form with `each`, `value` and `keep_when`
- pack settings `_gcpAssetScope`, `_gcpBillingProject`, `_gcpImageHistory`,
  `_gcpBackupHistory`, `_gcpSafeMetadataKeys`
- later, in its own design: `_pudl.same_resource` for shared identity across
  schemas
