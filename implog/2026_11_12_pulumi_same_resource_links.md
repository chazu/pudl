# Pulumi pack: same_resource links to live records (2026-11-12)

Issue: `pudl-7g8j.5` (closed). Design: `docs/design/2026-11-06-pulumi-pack.md`
§6, as-built note added.

## What was built

- **`pulumi.#Resource`** (`packs/pulumi/schema/pulumi/resource.cue`):
  - `_pudl.same_resource: [{kind: "k8s.object", key: uid:
    "live.metadata.uid"}, {kind: "gcp.resource", key: name: "live.name"}]`.
  - New field `live.name?: =~"^(projects|b)/"`.
- **`_pulumiGCPName`** (`packs/pulumi/schema/models/pulumi_pack.cue`): a
  per-type table that builds `live.name` from `id`. Listed types
  (`_pulumiGCPNamed`: network, subnetwork, address, global address,
  forwarding rule, global forwarding rule, router, service account in both
  spellings) keep the id. `gcp:container/cluster:Cluster` rewrites
  `locations/Z` to `zones/Z` when Z is zone-shaped, because a zonal
  cluster's `selfLink` says `zones/`. `gcp:storage/bucket:Bucket` becomes
  `b/<id>`. Other types get no `live.name`. The transform sets `live.name`
  only when the resource has a string `id` and the table yields a name.
- **Kind `gcp.resource`**: `same_resource_kind_gcp_resource` in both
  `packs/pulumi/schema/pudl/rules/pulumi_pack.cue` and
  `packs/gcp/schema/pudl/rules/gcp_pack.cue` (identical, so they unify
  when both packs are installed and doctor passes with either alone).
- **GCP pack keys** (`gcp.resource`, key `name`): `gcp.#Network`,
  `gcp.#Subnetwork`, `gcp.#Router`, `gcp.#Address` and
  `gcp.#ForwardingRule` from `selfLink` with `trim_prefix: _gcpComputeAPI`;
  `gcp.#GKECluster` from `selfLink` with the container API prefix trimmed
  (added to its existing `k8s.cluster` keys); `gcp.#Bucket` from `name`
  with `prepend: "b/"`; `gcp.#ServiceAccount` from `name`. No record
  changes.
- **Fixtures**: `resources-prod.json` gains a network, address, router,
  global forwarding rule, zonal GKE cluster, bucket and service account
  whose ids match the GCP fixtures, plus a firewall (an unlisted type).
  `resources-dev.json` gains a Deployment whose UID matches the CAI
  k8s-deployments fixture, with a planted env value in its outputs.
- **Docs**: both pack READMEs, the design as-built note and open question
  2.

## Not in the first set

- Projects and Artifact Registry repositories: the GCP pack does not
  collect them.
- DNS managed zones and record sets: their GCP records hold project and
  zone as separate fields, so no single-field key forms the relative name.
  Needs a multi-part key or a record change; decide with the user.
- Live verification of the Pulumi `id` formats (design open question 2)
  is still open: no `PUDL_PULUMI_TOKEN` was available in this session. The
  formats follow the provider's import documentation.

## Tests

- `cmd/pulumi_pack_resources_test.go`: `live.name` for a subnetwork, a
  zonal GKE cluster, a bucket and a service account; none for a firewall;
  the Deployment keeps only its metadata; counts updated (21 records, 15
  managed).
- `cmd/pulumi_pack_links_test.go` (new): installs both packs, runs nine GCP
  fixture collectors and `pulumi-fixture-resources`, then checks through
  `pudl show --json` that each of the eight linked state resources has
  exactly one partner of the expected GCP schema, that three resources
  (a subnet and a network absent from the GCP fixtures, and the firewall)
  have none, and that doctor reports 8 groups and no problems.
- `storedRecords` now lists with `--all` (the default list limit hid the
  21st record).
- `CGO_ENABLED=0 go test ./...` passes; `ratchets check` is clean.
