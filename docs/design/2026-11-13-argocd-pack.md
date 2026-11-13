# Argo CD pack: Applications, ApplicationSets and what they deploy

Status: proposed 2026-11-13. Issue: `pudl-avqx.3` (epic `pudl-avqx`).
Builds on the [Kubernetes pack](2026-11-05-kubernetes-pack.md),
[shared identity across schemas](2026-11-05-shared-identity-across-schemas.md)
and the [Pulumi pack](2026-11-06-pulumi-pack.md) (allow-list transforms,
`#HTTPObserve`).

## Problem

pudl sees workloads (`gcp.#K8sDeployment` today, the Kubernetes pack later)
but not the deploy tool that put them there. This design covers clusters
where Argo CD deploys workloads from Applications, with ApplicationSets that
generate them and Argo CD Image Updater that moves image tags. The
operations view (`pudl-avqx.7`) must answer, for each workload: which Application
deploys it, from which manifests repo, path and revision, what was deployed
last and when, and whether the app is synced and healthy.

Cloud Asset Inventory exports core Kubernetes kinds but not custom
resources, so `applications.argoproj.io` and `applicationsets.argoproj.io`
need their own collector. An Application spec can hold Helm values and
parameters, which can hold credentials. The collection must keep only what
it needs and must fail closed.

## Decisions

1. **kubectl, not the Argo CD API.** The collectors read the two CRDs with
   kubectl from the cluster that runs Argo CD, through the Kubernetes pack's
   command source and cluster record (`pudl-3q5.1`). Section 1 records the
   comparison. An API source on the same schemas is a later option.
2. **One schema per kind, records shaped by an allow list.** `argocd.#Application`
   and `argocd.#ApplicationSet` keep metadata, the source list, the
   destination and the status fields that facts need. The transform drops
   the rest. Helm values, `valuesObject` and parameter values are redacted
   with `sensitive_fields`, with an allow list for image parameters.
3. **Sources are a list.** The transform normalizes `spec.source` and
   `spec.sources` into one `sources` list with an `index` per element and
   the synced revision of that element. Facts and rules never read
   `spec.source` directly.
4. **Destination to cluster by host.** A rule derives the destination host
   from `spec.destination.server`; the in-cluster value
   `https://kubernetes.default.svc` resolves to the cluster the record was
   collected from. `spec.destination.name` gets a fact but no cluster link
   in the first slice (section 6).
5. **`deployed_by` joins on cluster, namespace, kind and name.**
   `status.resources` has no UID, so the workload edge is a rule join on
   the managed-resource fact, one rule per workload source.
6. **Write-back overrides are facts.** `spec.source.kustomize.images` and
   the allow-listed Helm image parameters are kept and projected, because
   with the `argocd` write-back method they hold the running tag and git
   does not.
7. **No repository node in the first slice.** `sourced_from` needs a node
   for a repo path, and pudl has no record or external node for one. The
   facts and the inspector rows carry the repo, path and revision now. The
   edge follows a small core change shared with `pudl-avqx.5` (section 7).

## 1. kubectl or the Argo CD API

Both read the same objects: the API server's `GET /api/v1/applications`
and `GET /api/v1/applicationsets` return the CRD objects from the
controller's informer cache, in the same shape, under `items`. Neither adds
data the other lacks for this design.

| | kubectl (`#CommandObserve`) | Argo CD API (`#HTTPObserve`) |
|---|---|---|
| Access needed | Cluster read on the two CRDs, which the Kubernetes pack already needs | A local account with the `apiKey` capability, an RBAC policy with `applications, get` and `applicationsets, get`, and the server's HTTPS endpoint reachable from where pudl runs |
| Read-only proof | Cluster RBAC; the same trust as the rest of the pack | Guard with GET `/api/v1/account/can-i/applications/{update,delete,sync}/*` all `no` and `/api/v1/session/userinfo` naming the account |
| Cluster record | The Kubernetes cluster record, set on every record, gives the host for the in-cluster destination for free | pudl must be told which cluster hosts the server (a setting) |
| Paging | None (`kubectl get -A`) | None; the list endpoint returns every app the account can see |
| Risk surface | Same as the Kubernetes pack | A second credential to manage; the API also serves `managed-resources` and `manifests` with live manifests, which pudl must never call |
| Works without cluster access | No | Yes, when a central Argo CD serves clusters pudl cannot reach |

**Choice: kubectl.** It needs no new credential, it reuses the cluster
record and the command source the pack already has, and its records enter
the same redaction and observation code path as the other Kubernetes
objects. The API is worth adding only when the Argo CD server is reachable
and the cluster is not; that is a follow-up (`argocd-api-*` collectors on
the same schemas, with the `can-i` guard above).

## 2. Layout

`packs/argocd/` mirrors the Pulumi pack: `bundle.go`, `README.md`,
`schema/argocd/*.cue` (package `argocd`), `schema/models/argocd_pack.cue`,
`schema/pudl/rules/argocd_pack.cue` and `argocd_graph.cue`, and
`populators/argocd/` (JSON fixtures). `internal/packs/packs.go` registers
it. The pack depends on the Kubernetes pack for the cluster record and the
`k8s.cluster` kind; `pudl doctor` reports the Argo CD pack installed without
it.

Settings (hidden fields, `schema/models/argocd_settings.cue`):

```cue
_argocdNamespaces: [...string] | *["argocd"]   // where Applications live
_argocdImageParameters: [...string] | *["image.name", "image.tag", "image.repository"]
```

Argo CD can watch Applications in any namespace ("apps in any namespace").
The collector lists all namespaces; `_argocdNamespaces` is only a filter the
workspace can set when other namespaces hold unrelated objects of the same
kind.

## 3. Collectors

Two collectors, `argocd-applications` and `argocd-applicationsets`, both
`#CommandObserve` with the Kubernetes pack's `each` (the current context's
cluster host, one element) and `set: cluster: "{{item}}"`:

```cue
runs: [{
	argv: ["kubectl", "get", "applications.argoproj.io", "-A", "-o", "jsonpath={.items}"]
	transform: {in: _, out: _argocdApplication & {_in: in}}
}]
```

`jsonpath={.items}` prints the list's items as one JSON array, which a run
decodes as one record per element. This answers the Kubernetes pack's open
question 1 for these collectors; `pudl-3q5.1` confirms it against a
reachable cluster, and the Argo CD collectors follow whatever that issue
settles on.

Observation: `scope: "argocd/applications"` and `argocd/applicationsets`,
`complete: false`, for the same reason as the Kubernetes pack (the scope is
fixed, the context is not). `pudl-u29` lifts this.

Fixture twins `argocd-fixture-*` read `populators/argocd/*.json` with `cat`
and the same transforms.

## 4. `argocd.#Application`

Identity: `[cluster, metadata.namespace, metadata.name]`. Shared identity:
`{kind: "k8s.object", key: uid: "metadata.uid"}`, so an Application seen
through another source later is the same node.

### Shape (allow list)

The transform keeps:

- `apiVersion`, `kind`, `cluster`, `metadata`: `name`, `namespace`, `uid`,
  `resourceVersion`, `creationTimestamp`, `labels`, `annotations`,
  `ownerReferences`, `finalizers`. Not `managedFields`.
- `project: spec.project`.
- `sources`: a list built from `spec.sources`, or `[spec.source]` when
  `spec.sources` is absent. Each element keeps `index` (its position),
  `repoURL`, `path`, `chart`, `targetRevision`, `ref`, `helm` (see
  redaction), `kustomize` (`images`, `namePrefix`, `nameSuffix`,
  `namespace`, `commonLabels`) and `directory.recurse`, `directory.include`,
  `directory.exclude`, plus `syncRevision` from `status.sync.revision`
  (single source) or `status.sync.revisions[index]`. Plugin sources keep
  `plugin.name` only; `plugin.env` and `plugin.parameters` are dropped.
- `destination`: `server`, `name`, `namespace`.
- `syncPolicy`: `automated.prune`, `automated.selfHeal`, `syncOptions`.
- `status`: `sync.status`, `sync.revision`, `sync.revisions`,
  `health.status`, `reconciledAt`, `sourceType`, `sourceTypes`,
  `operationState`: `phase`, `startedAt`, `finishedAt`, `operation.sync.revision`,
  `operation.initiatedBy`. `conditions` as `type` and `message`.
- `status.resources`: every element, `group`, `version`, `kind`, `namespace`,
  `name`, `status`, `health.status`, `requiresPruning`.
- `status.summary`: `images`, `externalURLs`.
- `status.history`: every element, `id`, `revision`, `revisions`,
  `deployStartedAt`, `deployedAt`, `initiatedBy.username`,
  `initiatedBy.automated`. The embedded `source` and `sources` are dropped:
  they repeat the Helm block of each past sync. A past source change is in
  git.

Dropped: `spec.ignoreDifferences`, `spec.info`, `spec.revisionHistoryLimit`,
`status.operationState.syncResult.resources` (one row per resource per sync;
`status.resources` is the current state), and everything else.

### Redaction

`sensitive_fields` on the schema, after the transform:

```cue
_argocdHelmSensitive: [
	"sources[*].helm.values",
	"sources[*].helm.valuesObject",
	"sources[*].helm.fileParameters[*].path",
	{each: "sources[*].helm.parameters[*]", value: "value",
		keep_when: {field: "name", in: _argocdImageParameters}},
	#"metadata.annotations."kubectl.kubernetes.io/last-applied-configuration""#,
]
```

`helm.valueFiles`, `helm.releaseName`, `helm.version`, `helm.skipCrds` and
`helm.ignoreMissingValueFiles` stay. `helm.parameters[*].name` stays for
every parameter, so an override is visible even when its value is not.

A hostile fixture plants values in `helm.values`, `helm.valuesObject`, a
parameter named `db.password`, `plugin.env`, a history entry's
`source.helm.parameters`, and the last-applied annotation. The test ingests
it and searches every stored byte for each planted value; it must find
zero, as the Pulumi pack's test does.

### Image Updater and write-back

Argo CD Image Updater reads `argocd-image-updater.argoproj.io/*` annotations
on the Application: `image-list`, `write-back-method` (`argocd`, the
default, or `git`), `write-back-target`, `git-branch`, and per-alias
`<alias>.update-strategy`, `<alias>.allow-tags`, `<alias>.helm.image-name`,
`<alias>.helm.image-tag`, `<alias>.kustomize.image-name`. The annotations
are kept whole (they hold no secrets) and three are projected.

With the `argocd` method, the updater patches the Application spec instead
of git: `spec.source.kustomize.images` gains `name=registry/repo:tag`
entries, and `spec.source.helm.parameters` gains the parameters named by
`<alias>.helm.image-name` and `<alias>.helm.image-tag` (`image.name` and
`image.tag` by default). The tag then lives only in the cluster. These are
why `kustomize.images` is kept whole and why `_argocdImageParameters` is an
allow list: a workspace that names other parameters in the alias annotations
adds them to the setting. `status.summary.images` holds the images Argo CD
actually rendered, and is the authoritative answer for "what runs".

With the `git` method, the updater commits `.argocd-source-<app>.yaml` to
`write-back-target` on `git-branch`. The Application spec does not change;
`status.sync.revision` moves instead.

### Facts

```
argocd_app(resource_id, cluster, namespace, name, project, sync_status,
           health_status, sync_revision, reconciled_at, automated, self_heal, prune)
argocd_app_source(resource_id, index, repo_url, path, chart, target_revision,
                  ref, sync_revision)                        -- each: sources[*]
argocd_app_destination(resource_id, server, name, namespace) -- "" defaults
argocd_app_resource(resource_id, group, version, kind, namespace, name,
                    status, health)                          -- each: status.resources[*]
argocd_app_image(resource_id, image)                         -- status.summary.images[*]
argocd_app_history(resource_id, id, revision, deployed_at, automated)
                                                             -- each: status.history[*]
argocd_app_kustomize_image(resource_id, index, image)         -- sources[*].kustomize.images[*]
argocd_app_helm_parameter(resource_id, index, name, value)    -- value "[REDACTED]" unless allow-listed
argocd_app_image_updater(resource_id, image_list, write_back_method, write_back_target)
argocd_app_owner(resource_id, kind, name, uid)                -- metadata.ownerReferences[*]
```

`argocd_app_history` keeps `revision` only; multi-source `revisions` stays
in the record. `automated` and the other booleans default to `false`;
`write_back_method` defaults to `"argocd"`, which is the updater's default.

## 5. `argocd.#ApplicationSet`

Identity and shared identity as for Applications. The transform keeps
`metadata` (as above), `spec.goTemplate`, `spec.syncPolicy`, and:

- `template`: `metadata.name`, `metadata.labels`, `metadata.annotations`,
  and `spec` through the same allow list and `sources` normalization as an
  Application (section 4). Template strings such as `{{path.basename}}`
  are kept as text.
- `generators`: a summary per generator, not the generator itself:
  `type` (the one key the generator has: `git`, `list`, `clusters`,
  `pullRequest`, `scmProvider`, `matrix`, `merge`, `plugin`), and for `git`
  the `repoURL`, `revision` and the `directories[*].path` or
  `files[*].path` patterns; for `list` the element count; for `clusters`
  the `selector`. `matrix` and `merge` carry the same summary for their
  child generators, one level deep. Generator `template` overrides and
  `list.elements` are dropped.
- `status.conditions` as `type` and `message`.

Redaction: `_argocdHelmSensitive` with `template.spec.` in front of each
path.

Facts:

```
argocd_appset(resource_id, cluster, namespace, name, generator_types)
argocd_appset_git_generator(resource_id, repo_url, revision)
```

The `generated_by` edge from an Application to its ApplicationSet comes
from `argocd_app_owner` with `kind: "ApplicationSet"` and the owner `uid`,
joined to the ApplicationSet's own `metadata.uid` through the
`k8s.object` key.

## 6. Destination and cluster links

The destination can be a server URL or a cluster name:

```cue
// The host an Application deploys to. The in-cluster server is the
// cluster the record came from; any other server is its URL's host.
argocd_app_deploys_to_host: [
	{head: {rel: "argocd_app_target_host", args: {resource_id: "$R", host: "$H"}}
	 body: [{rel: "argocd_app", args: {resource_id: "$R", cluster: "$H"}},
	        {rel: "argocd_app_destination", args: {resource_id: "$R", server: "https://kubernetes.default.svc"}}]},
	{head: {rel: "argocd_app_target_host", args: {resource_id: "$R", host: "$H"}}
	 body: [{rel: "argocd_app_destination_host", args: {resource_id: "$R", host: "$H"}}]},
]
```

`argocd_app_destination_host(resource_id, host)` is a projected fact: the
transform writes `destinationHost` as the server URL's host with the scheme
and a `:443` port removed, as the Kubernetes pack's cluster record does for
its own `host`, and only when the server is not the in-cluster value. A
destination given by `name` yields no host fact. Resolving a name needs the
cluster Secrets in the Argo CD namespace (`argocd.argoproj.io/secret-type:
cluster`), which hold credentials; an allow-list collector for `data.name`
and `data.server` alone is a follow-up, not part of the first slice.

The target host joins the Kubernetes pack's cluster fact
(`k8s_cluster(resource_id, host)`); a linked GKE cluster is the same node
through the `k8s.cluster` key. The `same_resource` keys stay on the cluster
records; the Application declares none for the destination, because the
in-cluster value needs the join above and a key must be a path.

## 7. Graph

Edge types, declared in `argocd_graph.cue`:

| Type | Category | From | To | Meaning |
|---|---|---|---|---|
| `in_cluster` (built-in) | containment | Application, ApplicationSet | the cluster that runs Argo CD | Kubernetes object that lives in this cluster. |
| `generated_by` | containment | Application | ApplicationSet | Generated from this ApplicationSet's template. |
| `deploys_to` | dependency | Application | destination cluster | Sync creates this application's objects in this cluster. |
| `deployed_by` | dependency | workload | Application | Argo CD lists this object among the application's managed resources. |
| `sourced_from` | dependency | Application | repository path | Manifests come from this repository path at the target revision. |

`deployed_by`, one rule per workload source:

- Kubernetes pack workloads (`pudl-3q5.2`): `k8s_deployment(resource_id,
  cluster: $H, namespace: $NS, name: $N)` joined with
  `argocd_app_resource(resource_id: $A, kind: "Deployment", namespace: $NS,
  name: $N)` and `argocd_app_target_host(resource_id: $A, host: $H)`. The
  same for StatefulSets and DaemonSets.
- `gcp.#K8sDeployment`: the same join, with the cluster matched through
  `gcp_gke_cluster` and a new `gcp_gke_cluster_endpoint(resource_id, host)`
  fact in the GCP pack (public and DNS endpoints, the same paths as its
  `k8s.cluster` keys). The Argo instance label fact of `pudl-avqx.2` is a
  check on this join, not its basis: a label can be set by hand.

`sourced_from` is declared but not derived in the first slice (decision 7).
pudl's external nodes (`external.cidr`, `external.principal`) are fixed in
`internal/graphexport/external.go`; rules cannot name a node that no record
backs. The repository node is one decision for this pack and for
`built_from` in `pudl-avqx.5`: either a core relation that lets rules
declare a node (`graph_node(id, type, name)`), or a `git.repo` identity kind
with a record per repository. That decision needs its own issue, still to
be filed (the "new: Repository node" row in the Delivery table), and it
blocks both edges.

Display: `_pudl.graph.display` codes `APP` ("Argo CD application") and
`APPSET` ("Argo CD application set"), class `compute`. Both draw as nodes
(not `member_only`): an Application is a thing an operator looks for.

Inspector (`graph_detail`, section "Argo CD") on an Application, and on
each workload it deploys through `deployed_by`: repo, path, target
revision, synced revision, sync status, health, last deploy time
(`argocd_app_history` with the highest `id`), and the Image Updater
write-back method when the annotation is set.

## 8. Fixtures and tests

- `populators/argocd/applications.json`: a single-source Helm app with the
  in-cluster destination, a multi-source app (`ref: values` plus a chart)
  with a server URL destination, a kustomize app with Image Updater
  annotations and a written-back `kustomize.images` entry, and an app owned
  by an ApplicationSet. Managed resources name Deployments whose cluster,
  namespace and name match the GCP pack's Kubernetes fixture deployments.
- `populators/argocd/applicationsets.json`: a git directory generator and a
  matrix of clusters and list.
- Hostile fixture (section 4) and its zero-planted-bytes test.
- Rule tests: `argocd_app_target_host` for both destination forms,
  `deployed_by` against the GCP fixtures, `generated_by`.

## Delivery

| Issue | Work | Size |
|---|---|---|
| `pudl-avqx.3` | This design (done) | 2 hours |
| `pudl-3q5.1` | Kubernetes pack skeleton: cluster record, command source, `items` unwrap | existing |
| `pudl-avqx.4` | Pack: schemas, transforms, redaction, collectors, fixtures, facts, rules, edges, inspector rows; `gcp_gke_cluster_endpoint` in the GCP pack | about 1.5 days |
| new | Repository node: core relation or `git.repo` kind; then `sourced_from` here and `built_from` in `pudl-avqx.5` | about half a day |
| new | Destination by name: allow-list collector for Argo CD cluster Secrets (`data.name`, `data.server`) | about 2 hours |
| new | Argo CD API collectors on the same schemas, `#HTTPObserve` with the `can-i` guard | about half a day |

## Open questions

1. Whether `jsonpath={.items}` prints the array as JSON on every kubectl
   version in use (the Kubernetes pack's open question 1; `pudl-3q5.1`).
2. Whether the operations view wants `status.operationState.syncResult`
   per sync (one row per resource per sync) once history is in. Left out
   until a view needs it.
3. ApplicationSet `list.elements` can hold cluster names and namespaces the
   view could use; they are dropped because an element can hold anything.
   Revisit if a generator summary is not enough.

## Out of scope

- Argo CD Projects (`appprojects.argoproj.io`), repository and cluster
  Secrets, and RBAC. A project fact on the Application is enough for now.
- Notifications, rollouts and Argo Workflows.
- Sync drift checks (a policy family can read `argocd_app` later).
