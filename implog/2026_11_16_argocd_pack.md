# Argo CD pack: Applications, ApplicationSets and deploy edges (2026-11-16)

Builds `pudl-avqx.4` to `docs/design/2026-11-13-argocd-pack.md` (now
accepted), with the three requirements the owner added: the ApplicationSet
half and an end-to-end test of an Application it generates; a fact for the
`argocd.argoproj.io/tracking-id` annotation that checks the `deployed_by`
join; and `deploys_to` for a server-URL destination on another cluster.

## The pack (`packs/argocd`)

| Path | Contents |
| --- | --- |
| `bundle.go` | `argocdpack.Files` (embed) |
| `README.md` | Install, collectors, settings, records and redaction, facts, rules, graph, fixtures |
| `schema/argocd/application.cue` | `argocd.#Application` |
| `schema/argocd/applicationset.cue` | `argocd.#ApplicationSet` |
| `schema/argocd/redaction.cue` | `_argocdImageParameters`, `_argocdSourceSensitive`, `_argocdLastApplied`, `_argocdPresent` |
| `schema/models/argocd_shape.cue` | The allow-list transforms `_argocdApplication`, `_argocdApplicationSet` and their parts |
| `schema/models/argocd_pack.cue` | `argocd-applications`, `argocd-applicationsets`, `argocd-fixture-applications`, `argocd-fixture-applicationsets` |
| `schema/models/argocd_settings.cue` | `_argocdNamespaces` (default `["argocd"]`; `[]` keeps every namespace) |
| `schema/pudl/rules/argocd_pack.cue` | `argocd_app_target_host`, `argocd_cluster`, `argocd_workload_app`, `argocd_workload_app_confirmed`, `argocd_app_last_deploy`, inspector rows |
| `schema/pudl/rules/argocd_graph.cue` | `in_cluster`, `generated_by`, `deploys_to`, `deployed_by` edges; edge types incl. `sourced_from` |
| `populators/argocd/{applications,applicationsets}{,-hostile}.json` | Fixtures as `kubectl get -o json` Lists |

Registered as `argocd` in `internal/packs/packs.go` (after `k8s`); listed in
`docs/README.md`.

### Collectors

The live collectors are `#CommandObserve` with the Kubernetes pack's
`_k8sClusterEach` (`set: cluster: "{{item}}"`) and the argv of
`_k8sGet & {_resource: "applications.argoproj.io"}` (and
`applicationsets.argoproj.io`). Their transform is
`_argocdListTransform`: it unwraps the List, filters by
`_argocdNamespaces`, and shapes each item. The scopes are
`argocd/applications` and `argocd/applicationsets`, with `complete: false`.
The fixture twins `cat` the List and the hostile List, and set `cluster` to
the DNS endpoint of the GCP fixture cluster `batch-private`.

### Schemas

Identity is `[cluster, metadata.namespace, metadata.name]`. Shared identity
is `k8s.object` by `metadata.uid`. Display codes are `APP` and `APSET`.
The design said `APPSET`, but `_pudl.graph.display.code` allows 5
characters at most and the schema failed to load with it.

`sensitive_fields` redact `sources[*].helm.values`,
`sources[*].helm.valuesObject`, `sources[*].helm.fileParameters[*].path`,
every `sources[*].helm.parameters[*].value` whose name is not on
`_argocdImageParameters`, and the last-applied annotation. The
ApplicationSet uses the same paths under `template.spec.`.
`_argocdImageParameters` is in package `argocd`, not `models`: redaction is
schema metadata and cannot read a models setting.

Each kept Helm parameter carries `sourceIndex`, so
`argocd_app_helm_parameter` (each over `sources[*].helm.parameters[*]`) can
name its source. A fact relation cannot reach a parent element's field.

Facts are as in the design, sections 4 and 5. There are three additions:

- `argocd_appset.uid`, which `generated_by` joins on
- `argocd_appset_child_git_generator`, for git generators inside a matrix
  or merge
- `argocd_app_destination_host`, the projected `destinationHost`

`generator_types` is the top-level types, comma-separated. A matrix or
merge lists its children, e.g. `matrix(clusters,list)`.

### Rules and edges

- `argocd_app_target_host(resource_id, host)`: the record's `cluster` for
  `https://kubernetes.default.svc`, else `argocd_app_destination_host`. A
  named destination yields none (`pudl-avqx.9`).
- `argocd_cluster(resource_id, host)`: `k8s_cluster` or
  `gcp_gke_cluster_endpoint`. `in_cluster` and `deploys_to` join on it, so
  a remote destination reaches the GKE record of a cluster pudl has no kube
  context for.
- `argocd_workload_app(workload, application)`: `gcp.#K8sDeployment` joined
  through `gcp_gke_cluster`, `gcp_gke_cluster_endpoint`,
  `argocd_app_target_host` and `argocd_app_resource` (group `apps`, kind
  `Deployment`, namespace, name). This is the basis of `deployed_by`. No
  rule exists yet for Kubernetes pack workloads (`pudl-3q5.2`).
- `argocd_workload_app_confirmed(workload, application, via)`: the same
  join, where `gcp_k8s_deployment_argo_app` (instance labels or the
  tracking-id annotation) names the Application.
- `argocd_app_last_deploy`: `max` of `argocd_app_history.deployed_at`.
- Inspector rows (`graph_detail`, section "Argo CD") on an Application and
  on each workload it deploys: application, sync status, health, last
  deploy, image write-back, and per source (rank = index) repo, path,
  chart, target revision, synced revision. Every rule of a relation must
  have the same head keys, so single rows carry `rank: 0`.

## GCP pack

- `gke.cue`: `gcp_gke_cluster_public_endpoint` and
  `gcp_gke_cluster_dns_endpoint` (the `k8s.cluster` key paths). The rule
  `gcp_gke_cluster_endpoint(resource_id, host)` unions them. A projected
  relation reads one path, so the union is a rule.
- `k8s.cue`: `gcp_k8s_deployment_argo_tracking_id(application,
  tracking_id)` from the `argocd.argoproj.io/tracking-id` annotation. The
  application is the text before the first colon of
  `<app>:<group>/<kind>:<namespace>/<name>`. The rule
  `gcp_k8s_deployment_argo_app_tracking_id` adds it to
  `gcp_k8s_deployment_argo_app` with `via:
  "argocd.argoproj.io/tracking-id"`.
- Fixtures: `batch-private` gains a DNS endpoint in both the gcloud and
  the CAI GKE fixtures. The CAI deployment `catalog` gains a tracking-id
  annotation.

## Core

`pudl pack install` takes several names and installs them in order, so
`pudl pack install k8s argocd gcp` works. `--json` takes one name. The CLI
reference and the help golden file are regenerated.

## Tests

- `cmd/argocd_pack_test.go`: `TestArgoCDPackFixtureApplications` (records,
  `sources` normalization, per-source `syncRevision`, destination host,
  namespace filter, redaction in records and facts),
  `TestArgoCDPackFixtureApplicationSets` (template, generator summaries,
  `generator_types`, git generator facts),
  `TestArgoCDPackHostileFixturesPlantNothing` (every planted value is
  absent from every file under the workspace except the bundled fixtures).
- `cmd/argocd_pack_graph_test.go`: `TestArgoCDPackTargetHost` (both
  destination forms, the named destination), `TestArgoCDPackEdges`
  (`deploys_to` in-cluster and to another cluster by server URL,
  `deployed_by`, `generated_by`, `in_cluster`, a complete vocabulary, and
  the inspector rows on the workload of the ApplicationSet's Application),
  `TestArgoCDPackDeployedByChecks` (instance label and tracking-id
  confirmations, `gcp_gke_cluster_endpoint`).
- Updated: `TestGCPPackK8sDeploymentFixture` (the tracking-id fact),
  `TestPackListShowsInstallState`, the packs list test, `TestHelpJSONGolden`.

## Found, not fixed

The Pulumi pack's `graph_detail` rules mix head keys: `pulumi_detail_component`
has `rank` and the others do not. `pudl doctor` reports this as a Datalog rule
problem in any workspace with the Pulumi pack, and a query that depends on
`graph_detail` then fails. The Argo CD rules give every row a `rank`.

## Not done

No kubectl context could be reached in this session, so the live collectors
ran only against fixtures. A live check is still to do.
