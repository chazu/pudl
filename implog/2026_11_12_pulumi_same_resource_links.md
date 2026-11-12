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

## Live verification (2026-11-12)

In a scratch workspace outside the repo, with the read-only org token:

- `pulumi-stacks` listed every stack. A scratch one-stack model exported
  one stack (89 resources, 19 types, among them service accounts, buckets,
  projects, Artifact Registry repositories, a DNS zone, a service account
  key and the GCP provider) through `_pulumiResourceTransform`.
- Stored records held only the allow-listed keys. `live.name` was set on
  the 5 service accounts and 2 buckets only, in the expected form.
- Leak check: every string value of the raw export outside the kept
  fields (1166 values from `inputs`, `outputs` and the like, including the
  key's `privateKey`) was searched for in every stored byte of the
  workspace. None was present except as a substring of a kept value (a
  short account id inside its own URN). No private key block, password,
  base64 blob or secret envelope was stored; the only occurrences of the
  envelope key words were the model's own `reject_keys` list in the
  observation report. `pudl doctor`'s sensitive scrub was clean.
- Id formats: all stacks' exports were scanned in memory for the first-set
  types. Every `id` is the relative resource name; buckets are the bare
  name. No zonal GKE cluster exists, so the `zones/Z` rewrite is untested
  live. Projects, Artifact Registry repositories, DNS zones and record
  sets have the formats noted in the design (open question 2 answered).
- The scratch workspace and the raw export were deleted.

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
