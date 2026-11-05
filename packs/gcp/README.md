# GCP content pack

An installable set of CUE files for cataloging Google Cloud with PUDL:
draft schemas for the core inventory, `gcloud` collectors, single-resource
best-practice policies, cross-resource checks and offline fixtures. It
contains no vendor-specific Go.

```bash
pudl init
pudl pack install gcp
```

Installed files, under `.pudl/`:

| Path | Contents |
| --- | --- |
| `schema/gcp/{network,compute,gke,data,services,iam,artifacts}.cue` | Type schemas (package `gcp`), all `review: status: "draft"` |
| `schema/gcp/owners.cue` | Ownership: the project-to-team table the types share |
| `schema/gcp/policies.cue` | Best-practice policies |
| `schema/pudl/rules/gcp_pack.cue` | Datalog rules for the checks |
| `schema/models/gcp_pack.cue` | Collectors, and the optional live settings |
| `schema/models/gcp_assets.cue`, `gcp_iam.cue`, `gcp_images.cue` | Cloud Asset Inventory sources and the shape each type is stored in |
| `schema/models/gcp_pack_checks.cue` | The three check families |
| `populators/gcp/*.json` | Fixtures: gcloud-shaped output for `example-project` |
| `populators/gcp/kms-keys.sh` | Helper `gcp-kms-keys` runs: every key ring's keys in one project |
| `populators/gcp/iam-roles.sh` | Helper `gcp-iam-roles` runs: every custom role in one project, described |
| `populators/gcp/service-account-keys.sh` | Helper `gcp-service-account-keys` runs: every account's user-managed keys in one project |
| `populators/gcp/container-images.sh` | Helper `gcp-container-images` runs: every Docker image in one project, with its newest tagged versions |

## What it collects

Each collector is a `#CommandObserve` model. A live collector lists your
projects at run time (`gcloud projects list`), runs one `gcloud ... list
--format=json` per project, injects `project` into every record and routes the
records to one schema. Each declares a complete observation of scope
`gcp/<type>` (`fixture/gcp/<type>` for fixtures). If a project's run fails
(a missing permission, say), the other projects are ingested, the
observation is incomplete, and the collector fails with the project listed
under `populate.failed_inputs` in its report. A family checked with it is
blocked; checked later on its own, its checks over that observation are
unknown, not passing. A project whose API for the type is disabled is
different: gcloud reports `SERVICE_DISABLED`, which the live collectors
declare in `populate.empty_when`, so the project counts as having none of
the type. It is listed under `populate.empty_inputs`, and the observation
stays complete.

| Collector | gcloud | Schema | Identity | Permission (role) |
| --- | --- | --- | --- | --- |
| `gcp-networks` | `compute networks list` | `gcp.#Network` | `selfLink` | `compute.networks.list` (`roles/compute.networkViewer`) |
| `gcp-subnets` | `compute networks subnets list` | `gcp.#Subnetwork` | `selfLink` | `compute.subnetworks.list` (same) |
| `gcp-firewalls` | `compute firewall-rules list` | `gcp.#Firewall` | `selfLink` | `compute.firewalls.list` (same) |
| `gcp-routes` | `compute routes list` | `gcp.#Route` | `selfLink` | `compute.routes.list` (same) |
| `gcp-routers` | `compute routers list` | `gcp.#Router` | `selfLink` | `compute.routers.list` (same) |
| `gcp-addresses` | `compute addresses list` | `gcp.#Address` | `selfLink` | `compute.addresses.list`, `compute.globalAddresses.list` (same) |
| `gcp-instances` | `compute instances list` | `gcp.#ComputeInstance` | `selfLink` | `compute.instances.list` (`roles/compute.viewer`) |
| `gcp-service-accounts` | `iam service-accounts list` | `gcp.#ServiceAccount` | `email` | `iam.serviceAccounts.list` (`roles/iam.serviceAccountViewer`) |
| `gcp-iam-policies` | `asset search-all-iam-policies` per project; fallback `projects get-iam-policy P` | `gcp.#IAMPolicy` | `resource` | `cloudasset.assets.searchAllIamPolicies`; fallback `resourcemanager.projects.getIamPolicy` (`roles/iam.securityReviewer`) |
| `gcp-iam-roles` | `iam roles list` then `describe`, via `iam-roles.sh` | `gcp.#IAMRole` | `name` | `iam.roles.list`, `iam.roles.get` (`roles/iam.roleViewer`) |
| `gcp-service-account-keys` | `iam service-accounts keys list --managed-by=user` per account, via `service-account-keys.sh` | `gcp.#ServiceAccountKey` | `name` | `iam.serviceAccountKeys.list` (`roles/iam.serviceAccountKeyAdmin` or `roles/iam.securityReviewer`) |
| `gcp-gke-clusters` | `container clusters list` | `gcp.#GKECluster` | `selfLink` | `container.clusters.list` (`roles/container.clusterViewer`) |
| `gcp-sql-instances` | `sql instances list` | `gcp.#SQLInstance` | `selfLink` | `cloudsql.instances.list` (`roles/cloudsql.viewer`) |
| `gcp-buckets` | `storage buckets list` | `gcp.#Bucket` | `name` | `storage.buckets.list` (`roles/viewer`) |
| `gcp-run-services` | `run services list` | `gcp.#RunService` | project number, location, name | `run.services.list` (`roles/run.viewer`) |
| `gcp-dns-zones` | `dns managed-zones list` | `gcp.#DNSManagedZone` | `project`, `name` | `dns.managedZones.list` (`roles/dns.reader`) |
| `gcp-pubsub-topics` | `pubsub topics list` | `gcp.#PubSubTopic` | `name` | `pubsub.topics.list` (`roles/pubsub.viewer`) |
| `gcp-pubsub-subscriptions` | `pubsub subscriptions list` | `gcp.#PubSubSubscription` | `name` | `pubsub.subscriptions.list` (same) |
| `gcp-kms-keyrings` | `kms keyrings list --location=L` | `gcp.#KMSKeyRing` | `name` | `cloudkms.keyRings.list` (`roles/cloudkms.viewer`) |
| `gcp-kms-keys` | `kms keys list` per key ring, via `kms-keys.sh` | `gcp.#KMSKey` | `name` | `cloudkms.cryptoKeys.list` (same) |
| `gcp-container-images` | `artifacts repositories list`, `packages list`, then `docker images list --include-tags` per image, via `container-images.sh` | `gcp.#ContainerImage` | `name` (image path) | `artifactregistry.repositories.list`, `artifactregistry.packages.list`, `artifactregistry.versions.list`, `artifactregistry.tags.list` (`roles/artifactregistry.reader`) |

`roles/viewer` covers every collector. Each service's API must be enabled in
the project, or its collector fails.

To import gcloud output by hand, inject the project ID as the collectors do.
Every type requires `project` to be a project ID (not the project number or
display name), and claims its records with `@pudl(match)`, so gcloud output
without a valid project ID is rejected rather than cataloged:

```bash
gcloud compute firewall-rules list --format=json > fw.json
pudl import --path fw.json --set project=my-project   # gcp.#Firewall
```

Sensitive values are replaced with `[REDACTED]` before anything is stored:
Cloud Run environment values and creator/modifier e-mail annotations (secret
references are kept), GKE node metadata (startup scripts) and legacy
`masterAuth` credentials, Compute Engine instance metadata values (startup
scripts, user-data, and `kube-env` on GKE nodes; the keys are kept, and the
settings in `_gcpSafeMetadataKeys`, such as `enable-oslogin`, keep their
values), and
Pub/Sub push endpoints, which can embed tokens.

The schemas are drafts. Compare each with your own gcloud output, then set
`review: {status: "validated", by: "...", at: "YYYY-MM-DD"}`. Until then
policy results are marked provisional.

## Ownership

Every type declares `_pudl: owner`. A resource belongs to the team in its
`team` label (`labels.team`; `resourceLabels.team` for GKE,
`settings.userLabels.team` for Cloud SQL, `metadata.labels.team` for Cloud
Run), else to its project's team, else to nobody. Networks, subnets,
firewalls, routes, routers and key rings have no labels and are owned by
project only. Name each project's team in `.pudl/schema/gcp/owners.cue`:

```cue
_gcpProjectOwners: {"my-project": "platform"}
```

Check findings and `pudl policy show` then name each resource's owner;
`pudl check ... --owner platform` shows one team's findings, and
`pudl query resource_unowned` lists what no label or project names. The
fixture Pub/Sub topics carry `team: orders`; everything else in
`example-project` is unowned until you map the project.

## Policies

Single-resource rules are policies. PUDL evaluates them on every observation
and stores the results as facts (`pudl policy evaluate --json` lists them). A
constrained field that a record lacks makes the result `unknown`, unless the
rule uses `absent="violation"`. The pack uses it only where gcloud omits a
field to mean "off".

| Policy | Rule | Severity | Absent field |
| --- | --- | --- | --- |
| `#FirewallBestPractice` | `gcp-firewall-world-ingress`: no enabled INGRESS ALLOW from `0.0.0.0/0` or `::/0` | high | n/a |
| `#AddressBestPractice` | `gcp-address-unused`: an external address is `IN_USE` | low | n/a |
| `#BucketBestPractice` | `gcp-bucket-public-access-prevention`: `enforced` | high | violation: not enforced |
| | `gcp-bucket-uniform-access`: uniform bucket-level access on | medium | violation: ACLs in use |
| `#SQLInstanceBestPractice` | `gcp-sql-backups`: `backupConfiguration.enabled` | high | violation: no backups |
| | `gcp-sql-pitr`: `pointInTimeRecoveryEnabled` or (MySQL) `binaryLogEnabled` | medium | violation, reported on `databaseVersion` |
| | `gcp-sql-public-ip`: `ipv4Enabled: false` | medium | violation: public IP is the default |
| | `gcp-sql-world-authorized-network`: no authorized network `0.0.0.0/0` | critical | n/a |
| `#KMSKeyBestPractice` | `gcp-kms-rotation`: an `ENCRYPT_DECRYPT` key has `rotationPeriod` | medium | violation, reported on `purpose` |
| `#GKEClusterBestPractice` | `gke-private-nodes`: `privateClusterConfig.enablePrivateNodes` | high | violation: public nodes |
| | `gke-control-plane-access`: private endpoint or authorized networks | high | violation, reported on `endpoint` |
| | `gke-workload-identity`: a `*.svc.id.goog` workload pool | medium | violation: Workload Identity off |

Asymmetric KMS keys cannot rotate automatically and are not flagged. Internal
address reservations are free and are not flagged.

## Check families

Each family is a checks-only model. Its checks read the latest observation of
the named collectors (evidence `model:<collector>`, at most an hour old).

| Family | Check | Severity | Collectors |
| --- | --- | --- | --- |
| `gcp-network-hygiene-checks` | `policy-high`: any high or critical policy violation | fail | firewalls, addresses |
| | `policy-any`: any policy violation | warn | firewalls, addresses |
| | `nat-ip-not-in-use`: a Cloud NAT lists an address GCP reports `RESERVED` | fail | routers, addresses |
| `gcp-data-protection-checks` | `policy-high`, `policy-any` | fail, warn | buckets, sql-instances, kms-keys |
| `gcp-gke-posture-checks` | `policy-high`, `policy-any` | fail, warn | gke-clusters |
| | `private-nodes-without-private-google-access`: a private-node cluster's subnet lacks Private Google Access | fail | gke-clusters, subnets |
| | `secrets-key-unusable`: secrets encryption uses a key whose primary version is disabled or destroyed | fail | gke-clusters, kms-keys |

Name a family together with its collectors; they run first:

```bash
pudl check gcp-fixture-firewalls gcp-fixture-addresses gcp-fixture-routers \
  gcp-fixture-network-hygiene-checks --detailed-exitcode
```

The fixtures contain compliant and noncompliant resources, so every fixture
family exits 2 (a failing fail-severity check). `pudl model list` shows every
model; each `gcp-fixture-*` model has a live `gcp-*` twin.

## Waiving a finding

To accept a finding until a date, add a waiver to the family in
`.pudl/schema/models/gcp_pack_checks.cue`. A policy waiver names the policy,
the rule and the resource ID (`pudl policy show gcp.#FirewallBestPractice`
lists failing resources by ID):

```cue
#GCPNetworkHygieneChecks: _#GCPNetworkHygiene & {
	_prefix: "gcp-"
	waivers: [{
		policy:   "gcp.#FirewallBestPractice"
		rule:     "gcp-firewall-world-ingress"
		resource: "<resource ID>"
		owner:    "security"
		reason:   "public bastion until the VPN lands"
		created:  "2026-11-01"
		expires:  "2026-12-31"
	}]
}
```

`policy-high` and `policy-any` then report that firewall `waived`, with the
waiver, and the family no longer exits 2 for it. After 2026-12-31 the finding
counts again and the report flags the expired waiver. A waiver can instead
target a cross-resource check's witness, for example `{check:
"nat-ip-not-in-use", witness: {address: "<address selfLink>"}, ...}`. The
policy-high rules carry `severity` so a waiver covers only the high and
critical violations that check reports.
See [governance](../../docs/governance.md) for how waivers match and expire,
finding changes (`--fail-on new`), owners and CI exit codes.

## Run against live projects

The live collectors name no project. Each lists the projects your gcloud
credentials can see when it runs (`gcloud projects list --format=json`) and
repeats its command per project, with `{{item}}` replaced by the project ID.
The report records the projects it enumerated under `populate.inputs`. A
failing project is a failed input as above; a failing `gcloud projects list`
fails the collector with nothing ingested.

### Cloud Asset Inventory

Networks, subnets, firewall rules, routes, routers, addresses, GKE clusters,
Cloud Run services, Pub/Sub topics, service accounts, DNS zones, KMS key
rings and keys, custom roles and service account keys are collected through
Cloud Asset Inventory (CAI) search: one
search per type
for every organization your credentials can see (`gcloud organizations list`,
at run time), instead of one gcloud list command per project. It needs
`cloudasset.assets.searchAllResources` on the organization (or the
projects), plus a billing project with `cloudasset.googleapis.com` enabled
where you hold `serviceusage.services.use`.

When CAI cannot answer (the asset API disabled or unbillable, or permission
denied), the collector falls back to its per-project gcloud commands and the
report says `source: fallback` with the error. Both sources store identical
records: each type's records are normalized to one shape (no `kind`
differences, fingerprints, etags or gcloud-only fields, DNS visibility in
lower case, a service account's `disabled` always stated), so switching
creates no new versions. CAI lists KMS key rings and keys in every location;
the collectors keep `_gcpKMSLocations`, as the gcloud commands do. Custom
roles defined in an organization rather than a project, and Google-managed
service account keys, are not collected from either source.

IAM policies are searched per project (`search-all-iam-policies`), whatever
`_gcpAssetScope` says, because `cloudasset.assets.searchAllIamPolicies` is
often granted on some projects and not on the organization. The search finds
the policy of every resource in the project: the project's own, and those of
its buckets, service accounts, secrets, keys, topics and so on. A project that
refuses the search is a failed input; gcloud's `get-iam-policy` needs a
similar grant, so it is not tried instead. Only a billing project that cannot
be used switches to gcloud, which reads each project's own policy and not its
resources'. That fallback is partial: its observation is incomplete, so the
other policies are kept rather than marked gone (as with
`_gcpAssetScope: "off"`). Audit configs are not kept: the search omits them.

Other types stay on gcloud, because CAI's copy loses something checks use:
compute instances (CAI's metadata omits most keys, including settings such
as `disable-legacy-endpoints`, and lower-cases values), Cloud SQL instances
(an older API version without `settings.edition`), buckets (no
`publicAccessPrevention` for some), and Pub/Sub subscriptions (not yet
compared).

Container images are collected through gcloud only. CAI lists every image
version as its own asset (tens of thousands in a real organization), while
`gcp-container-images` keeps one record per image with only its newest
`_gcpImageHistory` (default 10) tagged versions, newest push first; untagged
versions are left out. Container Registry needs no collector of its own:
gcr.io is served by Artifact Registry repositories named `gcr.io`,
`us.gcr.io` and so on, which are listed with the others. Remote (cache) and
virtual repositories are skipped. gcloud reads all of an image's versions to
sort them (a few seconds for an image with thousands, about a minute for a
project of a few dozen images), so the collector's runs time out after 30 minutes
instead of 10.

### Settings

1. Optionally narrow the projects, widen the KMS locations or choose how CAI
   is used. Set them in a file of your own, so the pack's files stay
   identical to the bundle, e.g. `.pudl/schema/models/gcp_settings.cue`:

   ```cue
   package models

   _gcpProjectFilter: "labels.env=staging"   // any gcloud --filter; default "" (all)
   _gcpKMSLocations: ["global", "us-central1"] // default ["global"]
   _gcpExtraEmptyWhen: ["Billing is disabled for project"] // default []
   _gcpAssetScope: "projects"   // "organization" (default), "projects" (one search per listed project), or "off" (gcloud only)
   _gcpBillingProject: "my-billing-project" // --billing-project for CAI; default "" uses gcloud's billing/quota_project
   _gcpImageHistory: 5 // tagged versions kept per container image, newest first; default 10
   ```

   Key rings are listed per project and location; `gcp-kms-keys` lists the
   keys of every ring it finds there. A project whose API is disabled is
   always an empty input; `_gcpExtraEmptyWhen` adds errors you have decided
   also mean "none here", such as billing being disabled.
2. Check your credentials and the models:

   ```bash
   gcloud auth list
   gcloud projects list
   pudl model validate gcp-firewalls
   ```

3. Run a family with its collectors, for example:

   ```bash
   pudl check gcp-gke-clusters gcp-subnets gcp-kms-keys gcp-gke-posture-checks --detailed-exitcode
   pudl policy evaluate --json
   ```

A collector's observation is complete only for the projects gcloud listed (or
the organizations, through CAI) and the KMS locations you set. Keep fixture and live runs in separate workspaces if
you want `pudl query` (latest-known facts) to show only one of them.

## Graph

`pudl graph export` and `pudl ui` draw the pack's records with these
relationships (rules in `schema/pudl/rules/gcp_graph.cue`):

| Relationship | From → to |
|---|---|
| `in_network`, `in_subnet` | subnets, firewall rules, routes, routers → network; instances, GKE clusters → subnet |
| `peers_with` | network → peered network (state as a property) |
| `allows_ingress_from` | enabled ingress firewall rule, public Cloud SQL instance (authorized networks), GKE control plane (enforced authorized networks) → address range |
| `applies_to` | firewall rule → instances it targets (whole network, tag or service account) |
| `attached_to` | address → what uses it (instance, forwarding rule, Cloud NAT router) |
| `nat_ip` | Cloud NAT router → its external address |
| `private_ip_in`, `private_zone_for` | Cloud SQL private IP, private DNS zone → network |
| `runs_as` | instance, Cloud Run service, GKE cluster (each node pool's account; node pool as a property) → service account |
| `encrypted_by` | GKE cluster (secrets), Pub/Sub topic, bucket (default key), Cloud SQL instance (disk) → KMS key |
| `in_key_ring` | KMS key → its key ring |
| `key_of` | user-managed service account key → its service account |
| `routes_via` | route → its next hop: instance, VPN tunnel or internal load balancer |
| `subscribed_to`, `publishes_to` | subscription → its topic, and its dead-letter topic |
| `pushes_to` | push subscription → the endpoint it delivers to, a Cloud Run service when its URL matches |
| `runs_image` | Cloud Run service → the container image it runs (gcr.io references mapped to their Artifact Registry repositories) |
| `iam_binding` | member → the IAM policy of the project or resource that grants it (role as a property) |
| `grants_on` | IAM policy → the resource it is attached to (project, bucket, compute resource, service account, topic, KMS key) |

A reference to something the pack did not collect (a forwarding rule, a VPN
tunnel, the Compute Engine default service account, a project) is drawn as
"not in inventory". Node pools on the Compute Engine default account
(`default`), routes to the internet gateway, and images outside Artifact
Registry have no edge. A push endpoint is redacted when stored; its origin
(`pushConfig.pushEndpointOrigin`: scheme and host, no credentials, path or
query) is kept to match it to a Cloud Run service. A service mapped to a
custom domain does not match.

## Impact: what depends on this network

`gcp_depends_on_network(network, resource, kind)` lists every cataloged
resource that uses a cataloged VPC network: subnets, firewall rules, routes,
Cloud Routers, GKE clusters, Cloud SQL instances with a private IP on it, and
private DNS zones visible to it. Networks are in path form; resources are
selfLinks, or the zone name for DNS:

```bash
pudl query gcp_depends_on_network network=projects/example-project/global/networks/prod-vpc
```

Over the fixtures, prod-vpc has 16 dependents. `pudl query` reads
latest-known facts, so run the collectors first.

## Limits

- Each record is evaluated on its own. Effective firewall reachability
  (priorities, DENY rules, target tags, hierarchical policies) is not computed.
- GKE references a subnet by path, which joins; the top-level short
  `subnetwork` name does not.
- gcloud field shapes change between releases. Re-check the schemas when you
  upgrade gcloud.
