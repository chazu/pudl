# GCP collectors on Cloud Asset Inventory

**Date:** 2026-11-03

**Status:** Accepted 2026-11-03. The [Decisions](#decisions-2026-11-03) govern;
the body is the design built on them. Nothing is implemented. Follows the GCP content pack
(`packs/gcp/`) and the instance-metadata redaction of 2026-11-02.

## Decision

The GCP pack's collectors read Cloud Asset Inventory (CAI) search first and
fall back to the gcloud list commands they use today.

- **One collector per resource type**, each with its own typed schema. The
  existing `gcp-<type>` collectors keep their names and observation scopes and
  gain CAI as their first source. New types get new collectors the same way.
- **A catch-all `gcp.#Asset`** holds resource types that have no typed
  collector yet. It never claims a resource a typed collector covers.
- **Scope is configurable:** every organization the credentials can see,
  discovered at run time (one search per type), or a list of projects (one
  search per type per project). The pack holds no organization-specific
  value.
- **Each schema picks the shape that suits it.** Source data is transformed
  into that shape, and both sources produce identical records, so switching
  source never creates duplicates.
- **Redaction gains an allowlist** expressed in CUE. The first user is
  instance metadata: safe keys keep their values, every other value is
  redacted.

## Problem

The pack runs one gcloud list command per type per project. With N projects
and 18 types that is 18 times N commands per full refresh, and it covers only
the 18 types the pack has schemas for. A large organization holds far more asset
types than that (census below).

CAI search answers one org-wide question per type. It returns the full v1 API
object when asked for `versionedResources`. It needs only
`cloudasset.assets.searchAllResources`, which a regular (non-admin) account
holds at the org.

## What was verified (2026-11-03)

All with a regular (non-admin) account in a real organization, billed to a
project where that account holds `serviceusage.services.use`.

| Fact | Evidence |
|---|---|
| Org-wide resource search works | `search-all-resources --scope=organizations/<id>` returned resources from many projects |
| Full resource data comes back from search | `--read-mask='name,assetType,versionedResources'` returned full bucket and instance objects, version `v1` |
| IAM policy search works per project | `search-all-iam-policies --scope=projects/<billing-project>` returned bindings |
| IAM policy search is **not** granted at the org | `testIamPermissions`: `searchAllIamPolicies` on some projects only, not at the org |
| Every call needs a billing project | `serviceusage.services.use` there, and `cloudasset.googleapis.com` enabled there |
| The search query cannot filter on asset type | `--query='NOT assetType:...'` fails `INVALID_ARGUMENT: Unsupported field: 'assetType'` |
| Asset types are chosen only by include patterns | `--asset-types`, RE2, full match |
| Every include pattern must match a supported type | one pattern matching none fails the whole call (`No supported asset type matches`) |

Census: org-wide passes reading only `assetType` found far more types than the
pack covers, with per-version types (container image versions) the most
numerous.

Not yet verified:

- whether CAI omits secret-bearing instance metadata keys (one GKE node
  sample had only `block-project-ssh-keys` and `vm-dns-setting`);
- whether `gcloud storage buckets list --raw` prints the API shape CAI uses;
- CAI search quotas per minute for the billing project;
- the shape of `versionedResources` for Cloud Run, IAM and Kubernetes types.

## 1. Sources and fallback

Each collector has a primary source (CAI search) and, where one exists, a
fallback (today's gcloud command). Types with no gcloud list command are
CAI-only.

**Fallback runs when CAI is unavailable,** recognized by stderr patterns:
`SERVICE_DISABLED` and `USER_PROJECT_DENIED` on the billing project, and
`PERMISSION_DENIED` on the scope. Any other CAI failure is a failed input, as
today. A fallback that also fails is a failed input.

**Implemented 2026-11-03 (`pudl-q4v.3`)** as `populate.fallback: {when,
observe}`, with one change from the first draft: the fallback replaces the
**whole** populate rather than only the failed input. The primary and
fallback usually fan out differently (organizations versus projects), so
there is no per-input correspondence, and replacing everything never mixes
two sources' records. The first matching failure stops the primary, which
also saves the remaining calls. The fallback is also used when the primary's
`each` fails with a matching error or lists no inputs (credentials that see
no organization). The check report carries `populate.fallback` with the
reason and trigger. See `docs/observation-scope.md#fall-back-to-another-source`.

A helper script that tries CAI and then gcloud would have needed no product
change, but the report could not say which source answered, and a silent
fallback hides an access regression.

## 2. Scope

The pack holds no organization-specific value: no org ID, project ID or
billing project. New settings in `packs/gcp/schema/models/gcp_pack.cue`:

```cue
// "organization": one search per type per organization the credentials
// can see, discovered at run time.
// "projects": one search per type per project gcloud lists
// (_gcpProjectFilter applies).
_gcpAssetScope: *"organization" | "projects"
// Passed as --billing-project when set. Empty uses gcloud's own
// billing/quota_project, so a workspace sets it in its own settings file
// or in gcloud config, never in the pack.
_gcpBillingProject: string | *""
```

**The organization is discovered at run time,** with an enumeration like
the project list:

```cue
_gcpOrgList: {argv: ["gcloud", "organizations", "list", "--format=json"], select: "name"}
// each run: --scope={{item}}, i.e. organizations/<id>
```

Credentials that see several organizations search each. Credentials that see
none (projects outside any organization) get an empty enumeration; the
report says so, and project mode is the way to collect them.

- Organization mode costs one search per type plus pages (500 per page).
- Project mode costs one search per type per project. It is the only mode
  for IAM policy search where that permission is not granted at the org.
- The fallback always fans out per project, as today.

The observation scope stays `gcp/<type>`. A narrower project filter already
makes "complete" mean "complete for the listed projects"; this does not
change.

## 3. Transforms

Every source record passes through a transform before routing. A transform
produces the schema's shape, or drops the record.

**Proposed: transforms written in CUE,** per run:

```cue
#CommandRun: {
	// ... existing fields ...
	// transform maps each printed record (in) to the record stored (out).
	// out: null drops the record.
	transform?: {in: _, out: _}
}
```

The CAI transform for an instance is roughly
`out: in.versionedResources[0].resource & {project: ...}`. The gcloud
transform for the same type is usually the identity, because gcloud prints
the API shape.

CUE keeps transforms declarative and testable with fixtures, and needs no jq
on the machine. The risk was cost: CUE evaluation per record. Measured
2026-11-03 (`pudl-q4v.2`): about 0.9 s per 5,000 synthetic 3 KB instance
records, with heap flat at about 5 MB over 20,000. Compiling each record as
CUE source leaked about 54 KB per record in the CUE runtime, so records are
built from a JSON syntax tree instead (which also keeps numbers exact).
Implemented as designed, except that `out` is optional in the schema and
required by the loader.

**Shape per type:** the v1 REST API object, unless a type has a reason
otherwise. That is what CAI returns and what most gcloud list commands print.

| Type | CAI shape | gcloud shape | Proposal |
|---|---|---|---|
| Compute, GKE, SQL, DNS, KMS, Pub/Sub, IAM service accounts | API v1 | API v1 | API v1, no transform beyond unwrapping |
| Bucket | API v1 (camelCase) | `gcloud storage` (snake_case) | API v1; verify `gcloud storage buckets list --raw`, else transform |
| Project IAM policy | per-resource policy | `get-iam-policy` per project | the per-resource shape (§5); transform the gcloud output into it |
| Cloud Run | verify | Knative v1 | decide after checking `versionedResources` |

Identity fields stay what the schemas declare (mostly `selfLink`). Both
sources must produce them, so the same resource gets the same entry whichever
source answered. A fixture test per type asserts this.

## 4. Redaction allowlist

The path grammar (`internal/fieldpath`) cannot select a list item by a
sibling's value, so today instance metadata is redacted entirely. Proposed
CUE:

```cue
_pudl: sensitive_fields: [
	"some.plain.path",
	{
		// Redact each item's value unless its key is listed.
		each:  "metadata.items[*]"
		value: "value"
		keep_when: {field: "key", in: _gcpSafeMetadataKeys}
	},
]

_gcpSafeMetadataKeys: [...string] | *[
	"block-project-ssh-keys", "enable-oslogin", "enable-oslogin-2fa",
	"serial-port-enable", "vm-dns-setting", "disable-legacy-endpoints",
	"enable-guest-attributes", "google-logging-enabled",
	"google-monitoring-enabled", "cluster-name", "cluster-location",
]
```

Unknown keys stay redacted, so a new secret-bearing key is safe by default.
Every consumer of sensitive paths must learn the new entry form:
`internal/redact`, `internal/validator` (parsing and strict checks),
`internal/scrub` and `internal/repair/sensitive_scrub.go` (retroactive scrub),
`internal/doctor/secret_check.go` and `internal/secretscan`. A schema whose
entry cannot be parsed still fails closed.

The same form applies to the pack's other blanket redactions (Cloud Run env
values, GKE node metadata). Each is reviewed separately.

## 5. Types and schemas

**Skipped:** Docker image versions, Container Registry image versions, pods,
replica sets, secret versions, Cloud Run revisions. Kubernetes and GKE
built-in defaults (`system:` roles and bindings, objects in `kube-system`,
`gke-*` and `gmp-*` namespaces, default service accounts) are dropped by the
transform.

**Images:** one record per image (`gcp.#ContainerImage`), with its repository,
newest digest, and the newest `_gcpImageHistory` (default 10) tagged versions,
each with digest, tags and push time. Untagged versions are left out. Source:
Artifact Registry repositories from CAI, then per repository
`gcloud artifacts docker images list <repo>/<image> --include-tags
--sort-by=~UPDATE_TIME --limit=<n>`, through a helper script like
`populators/gcp/kms-keys.sh`. Container Registry images get the same record
from `gcloud container images list-tags`.

**Cloud SQL backups:** the newest `_gcpBackupHistory` (default 10) per
instance, for both `sqladmin.googleapis.com/Backup` and `BackupRun`. A
per-record transform cannot rank records, so this uses a helper script over
`gcloud sql backups list --instance=<i> --limit=<n>` per instance (verify
that it lists newest first). CAI supplies the instance list.

**Cataloged in full:** DNS record sets and enabled services
(`serviceusage.googleapis.com/Service`), though each runs to thousands.

**IAM:**

- `gcp.#IAMPolicy`: one record per resource with a policy, from
  `search-all-iam-policies`, in project mode (§2). Facts:
  `gcp_iam_member(resource, role, member)`, one per member of each binding.
  Fallback for project policies: `gcloud projects get-iam-policy`,
  transformed. Resource-level policies have no practical fallback; the report
  lists those projects as failed inputs.
- `gcp.#IAMRole`: custom roles (`iam.googleapis.com/Role`).
  Predefined roles are not in CAI.
- `gcp.#ServiceAccountKey`: key metadata only (age and type matter for
  checks). CAI holds no private key material; verify, and declare
  `publicKeyData` and anything else that looks like key material sensitive
  regardless.

**Every new schema declares its secrets before it routes a record.** Known
risks: Cloud Build trigger substitutions, App Engine and Cloud Functions
environment variables, monitoring notification-channel labels (webhook URLs,
tokens), API key strings (`apikeys.googleapis.com/Key`), Firebase config.

**Fixtures are synthetic.** No record from the real org is committed, even
redacted.

## 6. The `gcp.#Asset` fallback

- **Identity:** the CAI `name` (full resource name). Fields: `name`,
  `assetType`, `project`, `folders`, `location`, `labels`, `state`,
  `parentFullResourceName`, `createTime`, `updateTime`. No `resource.data`:
  an unreviewed type's data may hold secrets.
- **Never overlaps a typed collector.** It searches with a checked include
  pattern set that excludes skipped types, and its transform drops every
  asset type a typed collector covers. The pattern set is generated from the
  pack's type map and tested (each pattern must match a supported type, §
  "What was verified").
- **Never claimed by inference:** routed explicitly, with a `match` on a
  `name` starting with `//`, so a gcloud import can never land in it.
- **In the UI:** a single generic class, hidden unless asked for. Doctor's
  Graph Vocabulary check must stay clean.

A type graduates by getting a typed schema and collector; its old
`gcp.#Asset` entries are then superseded on the next observation.

## 7. Delivery

Each step is a beads issue and ends green (`CGO_ENABLED=0 go test ./...`, the
GCP pack test, doctor clean on the example).

1. **Redaction allowlist** (§4), with the instance metadata list. About a
   day.
2. **Transforms in CUE** (§3), with a measured cost on 5,000 records. About a
   day; a day more if helper scripts are needed instead.
3. **Source fallback** (§1) and the scope settings (§2). About a day.
4. **Move the 18 existing types to CAI first**, bucket shape decided, fixture
   test that both sources give identical entries. About two days.
5. **IAM policies and roles** (§5), project mode. About a day.
6. **Images** (§5). About a day.
7. **New types in batches,** about a day per batch: (a) org, folders, org
   policies, firewall policies, Binary Authorization, access context,
   enabled services; (b) load balancing, network, DNS record sets;
   (c) data stores (AlloyDB, Redis, Firestore, SQL backups); (d) build and
   deploy (Cloud Build, App Engine, Functions, Run domain mappings);
   (e) observability (log sinks and metrics, alert policies, dashboards,
   notification channels); (f) Kubernetes objects (§8).
8. **`gcp.#Asset` fallback** (§6). About a day.

Total: about 2 to 3 weeks of work.

## 8. Kubernetes objects and shared identity

GKE exports Kubernetes objects to CAI. Their GCP representations belong in
the GCP pack (`gcp.#K8sDeployment` and so on), identified by the CAI full
name, e.g. `//container.googleapis.com/projects/<p>/locations/<l>/clusters/<c>/k8s/namespaces/<ns>/apps/deployments/<d>`.

A Kubernetes pack, later, will see the same objects through kubectl under its
own schemas. The two must be recognizable as one resource without merging
their records. That needs a product feature, designed separately: a schema
declares a cross-schema key, e.g.

```cue
_pudl: same_resource: {
	kind: "k8s.deployment"
	key: {cluster: "...", namespace: "...", name: "..."}  // field paths
}
```

and pudl relates entries of different schemas whose `kind` and key values
match (a `same_resource(a, b)` relation in datalog). The hard part is a
common cluster key: CAI names the GKE cluster, while kubectl knows a context.
`docs/VISION.md` lists cross-provider correlation as a goal; this would be
its first use. Until it exists, the GCP pack's Kubernetes schemas stand
alone.

## Follow-up designs

1. Shared identity across schemas (§8):
   [2026-11-05-shared-identity-across-schemas.md](2026-11-05-shared-identity-across-schemas.md).

## Findings during delivery

- **Live parity, 2026-11-03.** Networks, subnets, firewalls, routes, routers
  and addresses: CAI's copy equals gcloud's except `kind`, fingerprints and
  `x_gcloud_*`, which the shape normalizes. These six moved to CAI.
- **Compute instances are not equivalent.** CAI's metadata is a filtered
  view (on GKE nodes only a few keys, without settings such as
  `disable-legacy-endpoints`) with lower-cased values; Shielded VM
  certificates use standard instead of URL-safe base64. Instances stay on
  gcloud (decision 13).
- **Second parity round, 2026-11-03.** GKE clusters (gcloud adds node-pool
  `kubeletCertInfo`), Cloud Run services and Pub/Sub topics (identical),
  service accounts (gcloud adds `etag`; CAI omits `disabled: false`), DNS
  zones (CAI omits nested `kind`s and upper-cases `visibility`), KMS key
  rings and keys (identical; CAI covers every location, so the live search
  keeps `_gcpKMSLocations`) moved to CAI. Verified end to end with pudl on a
  live project: CAI then gcloud stores 0 new records for all 13 CAI types.
- **Stay on gcloud:** Cloud SQL (CAI serves v1beta4 without
  `settings.edition`, `geminiConfig`, `serverCaCert`; `settingsVersion`
  differs), buckets (even `--raw` differs in formats, and CAI lacks
  `publicAccessPrevention` for some buckets), project IAM policies (same
  role/member pairs, no `auditConfigs`; search is not granted everywhere;
  see pudl-q4v.5), Pub/Sub subscriptions (none to compare).

- **IAM, 2026-11-03 (`pudl-q4v.5`).** `search-all-iam-policies` per
  project: one result per resource, `policy` holding only `bindings`, the
  project named by number, the resource by full name (a project's by ID).
  The project policy's bindings equal `get-iam-policy`'s, order included.
  Some projects allow the search and the others refuse both the
  search and `get-iam-policy`, so a refused project is a failed input, not
  a fallback trigger; only `SERVICE_DISABLED` and `USER_PROJECT_DENIED`
  switch to gcloud. Because that fallback sees only project policies it is
  partial (new `populate.fallback.partial`): its observation is incomplete.
  Custom roles: CAI adds `groupName`/`groupTitle` and omits `stage` when it
  is `ALPHA`; gcloud's `describe` adds `etag` (its `list` omits
  permissions). Organization roles are dropped (no project). Service
  account keys: user-managed keys identical in both sources; Google-managed
  keys differ entirely (rotation) and are not collected. CAI holds no key
  material.

- **Images, 2026-11-04 (`pudl-q4v.6`).** Collected through gcloud only,
  per project: the repository list from `gcloud artifacts repositories
  list`, not CAI (one call per project saved, at the cost of org discovery
  and fallback in the helper). No separate Container Registry listing: the
  2025 migration made gcr.io, us.gcr.io, eu.gcr.io and asia.gcr.io Artifact
  Registry repositories, which the listing already covers. Versions sort by
  `CREATE_TIME` (push time); on migrated images `UPDATE_TIME` is the
  migration date for every version. gcloud reads all versions to sort them:
  tens of seconds for an image with many versions at the default page size, a few at
  `--page-size=1000`, which the helper uses; about a minute per live
  project. Runs time out after 30 minutes. Remote and
  virtual repositories are skipped.

## Decisions (2026-11-03)

Made with the user during the access research:

1. Collect through CAI search, billed to a project where the API is enabled
   and the collecting account holds
   `roles/serviceusage.serviceUsageConsumer`.
2. Instance metadata: an allowlist of keys whose values are kept, expressed
   in CUE; everything else redacted.
3. One collector per asset type; a typed schema for every real resource type
   (IAM members, roles, buckets, everything). A `gcp.#Asset` fallback is
   acceptable if it disturbs nothing else.
4. Searches can be scoped to projects; collectors fall back to gcloud
   commands when CAI is unavailable.
5. Each schema uses the shape that best fits pudl's needs; source data is
   transformed to it.
6. Images: names and tags, history capped (newest 10 tagged versions per
   image). Docker and Container Registry versions are not cataloged one by
   one.
7. Skip pods, replica sets and secret versions; catalog deployments.
8. Do not catalog Kubernetes and GKE built-in system defaults.
9. The organization is discovered at run time. The pack contains no
   organization-specific information.
10. Kubernetes objects from CAI get GCP representations in the GCP pack. A
    Kubernetes pack comes later, with a way to say that resources of
    different schemas share an identity.
11. Catalog all DNS record sets and enabled services.
12. Cloud SQL backups: keep the newest 10 per instance.
13. Compute instances stay on gcloud: CAI's copy of their metadata is lossy.
    Each type moves to CAI only after a live parity check.
