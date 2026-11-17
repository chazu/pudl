# Kubernetes pack: Deployments, StatefulSets, DaemonSets (2026-11-16)

`pudl-3q5.2`. The workload slice of the Kubernetes pack
(`docs/design/2026-11-05-kubernetes-pack.md`, Objects). Before it, pudl saw
workloads only through the GCP pack's `gcp-k8s-deployments` (Deployments,
GKE only), so a StatefulSet such as an Argo CD application controller was
invisible. `pudl query k8s_statefulset` and `k8s_daemonset` now list them
per cluster.

## Schemas (`packs/k8s/schema/k8s/workloads.cue`)

`k8s.#Deployment`, `k8s.#StatefulSet`, `k8s.#DaemonSet` (draft), sharing
`_#K8sWorkload` (`apiVersion: "apps/v1"`, `cluster`, `metadata` with name,
namespace and uid, `spec`, `status`) and `kind` as the match field.

- Identity: `["cluster", "metadata.namespace", "metadata.name"]`.
- `same_resource: [{kind: "k8s.object", key: uid: "metadata.uid"}]`, as
  `gcp.#K8sDeployment` declares.
- `sensitive_fields` (`_k8sWorkloadSensitive`): container and init container
  `env[*].value`, and the
  `kubectl.kubernetes.io/last-applied-configuration` annotation.
- Facts: `k8s_deployment(cluster, namespace, name, replicas)`,
  `k8s_statefulset(cluster, namespace, name, replicas)` (replicas default
  1), `k8s_daemonset(cluster, namespace, name)`.

Graph display info and `in_cluster` edges are left to `pudl-3q5.4`.

## Collectors (`packs/k8s/schema/models/k8s_workloads.cue`)

| Model | Reads |
| --- | --- |
| `k8s-deployments`, `k8s-statefulsets`, `k8s-daemonsets` | `kubectl get <kind>.apps -A -o json` on the current context's cluster; `complete: false` |
| `k8s-fixture-deployments`, `-statefulsets`, `-daemonsets` | `populators/k8s/<kind>.json` and `<kind>-hostile.json`; `complete: true` |

Built on the skeleton's shapes: `each: _k8sClusterEach`, `set: cluster:
"{{item}}"`, `(_k8sGet & {_resource: ...}).argv`. New shared pieces, all in
package `models`:

- `_k8sSystemNamespace`: the regexp of skipped namespaces (as the GCP pack).
- `_k8sOmit`: copy a struct without the named fields.
- `_k8sPodTemplate`: a pod template with every annotation value replaced by
  `[REDACTED]`; keys stay.
- `_k8sWorkload`: one record from a kubectl item, without
  `metadata.managedFields`, its template through `_k8sPodTemplate`.
- `_k8sWorkloadTransform`: unwrap the List, skip system namespaces, shape.
- `_k8sWorkloadLive`, `_k8sWorkloadFixture`, `_k8sFixtureHost`.

## Fixtures (`packs/k8s/populators/k8s`)

On the fixture cluster `198.51.100.30` (GCP `prod-private`). Deployments
`shop/checkout` and `shop/catalog` carry the UIDs of the GCP fixture
deployments; every kind has objects in system namespaces, which are
skipped. The hostile fixtures plant `FAKE-K8S-*` in env values, init
container env values, the last-applied annotation, pod template
annotations, `managedFields` and a system namespace object, next to
`valueFrom` references, which are kept.

## Tests (`cmd/k8s_pack_workloads_test.go`)

- `TestK8sPackFixtureWorkloads`: facts per kind with their cluster, system
  namespaces skipped, `managedFields` dropped, env values redacted, the two
  shared deployments one `k8s.object` group with `gcp.#K8sDeployment`, and
  `pudl doctor`'s same-resource check at 3 groups.
- `TestK8sPackHostileWorkloadsPlantNothing`: each hostile record stored and
  redacted; no `FAKE-K8S` byte under the workspace (`requireNoPlanted`).
- `TestK8sPackLiveWorkloads`: the live collectors against a fake `kubectl`
  on `PATH`; each runs `get <kind>.apps -A -o json`.

Not done: a run against a live cluster (not permitted in this session), and
the `deployed_by` join of Argo CD Applications onto these workloads (next to
`argocd_workload_app_gcp_deployment` in the Argo CD pack's rules).
