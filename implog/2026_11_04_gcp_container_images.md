# GCP pack: container images with capped tag history (pudl-q4v.6)

**Date:** 2026-11-04

## Work done

New collector `gcp-container-images` (and `gcp-fixture-container-images`):
one `gcp.#ContainerImage` record per Docker image in each project's
Artifact Registry repositories, with the newest `_gcpImageHistory` (default
10) tagged versions, newest push first. Untagged versions are left out.

| Collector | Schema | Identity | Source |
|---|---|---|---|
| `gcp-container-images` | `gcp.#ContainerImage` | `name`, the image path `LOCATION-docker.pkg.dev/PROJECT/REPO/IMAGE` | `container-images.sh` per project: `artifacts repositories list` (Docker, standard mode), `artifacts packages list` per repository, `artifacts docker images list IMAGE --include-tags --filter='tags:* AND package="IMAGE"' --sort-by=~CREATE_TIME --limit=N` per image |

Record shape (transform `_gcpShapes."container-images"`, `gcp_images.cue`):

```
{project, name, repository, location, digest?,
 versions: [{digest, tags, pushTime, buildTime?, sizeBytes?, mediaType?}]}
```

`digest` is the newest tagged version's; an image with only untagged
versions has none and an empty `versions`. Each version's `updateTime` and
resource name are dropped.

Facts: `gcp_container_image(repository, location)`,
`gcp_container_image_tag(tag, digest, push_time)` (one per tag of each kept
version). Graph display `IMG` / "Container image" / class `compute`. No
sensitive fields (names, digests, tags, times, sizes).

### Files

- `examples/gcp/schema/gcp/artifacts.cue`: `#ContainerImage`,
  `#ContainerImageVersion`.
- `examples/gcp/schema/models/gcp_images.cue`: the shape, the 30-minute
  run timeout.
- `examples/gcp/schema/models/gcp_pack.cue`: setting `_gcpImageHistory`
  (`int & >0 | *10`), schema and run entries, `_gcpRunTimeout` (per-type
  run timeout, applied to the live gcloud source), the two models.
- `examples/gcp/populators/gcp/container-images.sh`: the helper.
- `examples/gcp/populators/gcp/container-images.json`: synthetic fixture
  (helper output form): a gcr.io image with two tagged versions, a nested
  image (`web/frontend`), an image with no tagged version.
- `cmd/gcp_pack_images_test.go`: fixture records and facts; the live
  helper against a fake gcloud (calls, nested names, `_gcpImageHistory`).
- `cmd/gcp_pack_test.go`, `cmd/gcp_pack_inference_test.go`: the new type
  (its fixture is reshaped, so it is not imported as printed).
- `examples/gcp/README.md`, `docs/design/2026-11-03-gcp-cloud-asset-collectors.md`.

## Decisions and findings (live, 2026-11-04)

- **gcloud only, no CAI.** CAI holds each version as its own asset and
  cannot keep the newest per image; the repository list it could supply
  saves one call per project and would need its own org discovery and
  fallback. The design's "repositories from CAI" is dropped.
- **No Container Registry listing.** gcr.io is served by Artifact Registry
  repositories named `gcr.io`, `us.gcr.io`, `eu.gcr.io`, `asia.gcr.io`
  (created by the 2025 migration); listing Container Registry as well would
  store each image twice.
- **Sort by push time (`CREATE_TIME`), not `UPDATE_TIME`.** On migrated
  gcr.io images every version's `updateTime` is the migration date.
- **Cost.** gcloud has no server-side ordering for image versions: it reads
  all versions and sorts locally. One image with thousands of versions took tens of
  seconds at the default page size, a few with `--page-size=1000` (same 10 results), so
  the helper uses 1000. Live, with the helper: about a minute per
  project of a few dozen images. The run timeout is 30 minutes (default 10)
  for projects with hundreds of images.
- **Verified end to end** on those two projects' real output (fed to the
  fixture collector in a throwaway workspace, never committed): every image
  stored, at most 10 versions per image, newest first, none
  untagged; a rerun stores 0 new records.
- **Remote and virtual repositories are skipped.** None exist in the
  surveyed organization (all its Docker repositories are
  standard).
- gcloud prompts to enable a disabled API when stdin is open, and waits.
  pudl runs commands with stdin at `/dev/null`, so the prompt is answered no
  and the run fails with `SERVICE_DISABLED`, which `empty_when` matches.

## Public API

No Go API change. Pack surface: models `gcp-container-images`,
`gcp-fixture-container-images`; schema `gcp.#ContainerImage`; setting
`_gcpImageHistory`; facts `gcp_container_image`, `gcp_container_image_tag`.
