# Kubernetes content pack

Status: accepted 2026-11-05. Epic: `pudl-3q5`. Builds on
[shared identity across schemas](2026-11-05-shared-identity-across-schemas.md).

## Problem

pudl sees Kubernetes only through GCP (`gcp.#K8sDeployment`, from Cloud Asset
Inventory, GKE only) and through hand imports into the catch-all
`pudl/k8s.#Resource`, whose identity (kind, namespace, name) has no cluster.
A Kubernetes pack reads clusters directly with kubectl, for any distribution,
and links what it collects to the same cluster and objects seen through a
cloud provider.

## Decisions (2026-11-05, with the user)

1. **Current context only.** The live collectors read kubectl's current
   context. To collect another cluster, switch context and run again.
2. **Kinds:** Deployments, StatefulSets, DaemonSets, Services, Ingresses,
   Namespaces and Nodes. Pods and ReplicaSets are not collected (as in the GCP
   pack); neither are objects in system namespaces.
3. **Collectors first.** The first slice is the pack, its schemas, collectors,
   graph and links; a posture check family follows.
4. **Keys** (shared identity design, decision 3): the cluster record keys
   `k8s.cluster` by API server host; every object keys `k8s.object` by
   `metadata.uid`.

## Design

### Layout

`packs/k8s/` mirrors the GCP and AWS packs: `bundle.go` (embed),
`README.md`, `schema/k8s/*.cue` (package `k8s`, schemas `k8s.#Cluster`,
`k8s.#Deployment`, ...), `schema/models/k8s_pack.cue` (collectors),
`schema/pudl/rules/k8s_graph.cue`, `populators/k8s/` (helper scripts and
fixtures). Registered in `internal/packs/packs.go`. The built-in
`pudl/k8s.#Resource` stays the catch-all for hand imports.

### Cluster record: `k8s.#Cluster`

One record per run, printed by a helper (`populators/k8s/cluster.sh`) from
`kubectl config view --minify -o json` and `kubectl version -o json`:
`context`, `cluster` (kubeconfig cluster name), `server` (URL), `host` (the
server URL's host, scheme and `:443` removed, in the transform), and
`serverVersion`. Identity: `host`. `same_resource: [{kind: "k8s.cluster",
key: host: "host"}]` links it to `gcp.#GKECluster` (public or DNS endpoint).
A context that reaches a cluster by private IP does not link (private
endpoints repeat across VPCs).

### Objects

One collector per kind. Each enumerates the current cluster's host with
`each` (a one-element list from the cluster helper), sets it on every record
(`set: cluster: "{{item}}"`), and runs a helper that prints the `items` of
`kubectl get <resource> -A -o json` as a JSON array (runs decode an array as
one record per element; they cannot unwrap a List object).

- **Identity:** `[cluster, metadata.namespace, metadata.name]`, or
  `[cluster, metadata.name]` for Namespaces and Nodes. Stable when an object
  is recreated; the UID is not.
- **Shared identity:** `{kind: "k8s.object", key: uid: "metadata.uid"}`.
- **Shape** (CUE transform): drop `metadata.managedFields`; restore nothing
  (kubectl prints `apiVersion` and `kind`).
- **Skip** objects in `kube-system`, `kube-public`, `kube-node-lease`,
  `gke-*` and `gmp-*` namespaces, and those system namespaces themselves.
- **Redaction:** container and init container `env[*].value` in pod
  templates, and `metadata.annotations."kubectl.kubernetes.io/last-applied-configuration"`
  on every kind.
- **Facts:** per kind, the fields edges and later checks need (namespace,
  name, cluster, replicas, service type, ingress hosts, node providerID).

### Observation scope

A collector's scope is a fixed string, but the current context changes. A
complete observation scoped `k8s/deployments` would mark the previous
cluster's objects gone after a context switch. Until observations can be
scoped per enumerated input, the collectors declare `complete: false`:
nothing is marked gone, and deleted objects stay current until re-observed
or deleted by hand. Follow-up: per-input scope (`k8s/<host>/<kind>`).

### Graph

- `in_cluster` (built-in) from every object to its `k8s.#Cluster`, by the
  `cluster` field (the cluster's identity value).
- Display info for every type (codes: K8S, DEP, STS, DS, SVC, ING, NS, NODE).
- Linked records (a GKE cluster and its kube context; a deployment seen by
  GCP and by kubectl) are one node through shared identity.

### Fixtures and tests

Synthetic fixtures per kind and for the cluster, consistent with the GCP
pack's fixtures: the cluster's host equals the GCP fixture `prod-private`
public endpoint, and the GCP fixture deployments' UIDs appear in the
Kubernetes fixture deployments. A cross-pack test installs both packs, runs
both fixture collectors, and expects linked groups in `pudl graph export`.

## Delivery

| Issue | Work |
| --- | --- |
| skeleton (`pudl-3q5.1`) | Pack layout, registry, `k8s.#Cluster` and its collector, helpers, README |
| workloads (`pudl-3q5.2`) | Deployments, StatefulSets, DaemonSets |
| exposure and placement (`pudl-3q5.3`) | Services, Ingresses, Namespaces, Nodes |
| graph and links (`pudl-3q5.4`) | Display info, `in_cluster` edges, cross-pack link test with the GCP fixtures |
| checks (`pudl-5zh`) | Posture family: privileged containers, `:latest` images, no resource limits, run as root |
| observation scope (`pudl-u29`) | Per-input observation scope in populate, so these collectors can be complete |
| nodes to instances (`pudl-0qh`) | Link Nodes to GCE instances and EC2 by `spec.providerID` |

## Open questions

1. The helper's unwrap: `kubectl -o jsonpath='{.items}'` or a JSON tool.
   Verify against a reachable cluster in the skeleton issue (the current
   context timed out on 2026-11-05).
