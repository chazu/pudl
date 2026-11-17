# Kubernetes pack: graph display, in_cluster edges, cross-pack link test (2026-11-16)

`pudl-3q5.4`. The graph slice of the Kubernetes pack
(`docs/design/2026-11-05-kubernetes-pack.md`, Graph and Fixtures and
tests). Before it, the pack's objects were drawn as unknown types with no
edges; now each type has a code and a class, every object is in its
cluster, and an Ingress routes to its Services.

## Display info (`packs/k8s/schema/k8s/*.cue`)

Every type declares `_pudl.graph`: `name: "metadata.name"` (the cluster:
`name: "host"`, its identity) and `display`:

| Type | Code | Class |
| --- | --- | --- |
| `k8s.cluster` | `K8S` | compute (unchanged) |
| `k8s.deployment`, `k8s.statefulset`, `k8s.daemonset` | `DEP`, `STS`, `DS` | compute |
| `k8s.service`, `k8s.ingress` | `SVC`, `ING` | edge (as the GCP load-balancing types) |
| `k8s.namespace` | `NS` | admin |
| `k8s.node` | `NODE` | compute |

All classes are in the existing vocabulary. A merged node takes its labels
from the member whose schema sorts first, so `prod-private` and the shared
deployments keep the GCP pack's type and name.

## Edges (`packs/k8s/schema/pudl/rules/k8s_graph.cue`)

- `k8s_graph_<relation>_cluster`, one rule per object relation
  (`k8s_deployment`, `k8s_statefulset`, `k8s_daemonset`, `k8s_service`,
  `k8s_ingress`, `k8s_namespace`, `k8s_node`): `in_cluster` from the
  object to the `k8s_cluster` fact whose `host` is the object's `cluster`,
  joined on facts as the Argo CD pack's `argocd_graph_app_cluster` does.
- `k8s_graph_ingress_service`: `routes_to` from an Ingress to each Service
  in its namespace that `k8s_ingress_backend` names.

Both are built-in edge types; the pack declares none. No edge is drawn
without both records. With the GCP pack, the GCP rule
`gcp_graph_k8s_deployment_cluster` and this pack's rule give the same
`in_cluster` edge between the merged nodes; `pudl graph export` merges it
into one edge with both rules as evidence.

Not done: Namespace containment. `in_namespace` is not a built-in edge type
and this task adds none, so a Namespace does not contain its objects in the
graph.

## Test (`cmd/k8s_pack_graph_test.go`)

`TestK8sPackGraph` installs the gcp and k8s packs, runs
`gcp-fixture-gke-clusters`, `gcp-fixture-k8s-deployments` and every k8s
fixture collector, and checks the exported graph:

1. `prod-private` is one node with the `gcp.#GKECluster` and `k8s.#Cluster`
   records, and no cluster record has a node of its own.
2. `checkout` and `catalog` are one node each, with the `gcp.#K8sDeployment`
   and `k8s.#Deployment` records.
3. Every Kubernetes object (each k8s type, and the GCP-only deployment)
   has exactly one `in_cluster` edge, to the merged cluster node.
4. `requireCompleteVocabulary` passes and the export has no warnings.
5. No `in_cluster` or `routes_to` edge ends in a `ref:` placeholder. (Other
   GCP edges of these fixtures do: the KMS key of a GKE cluster is not
   collected.)

It also checks the `routes_to` edges (storefront to checkout and catalog,
and the hostile pair; not to `edge/edge-proxy`, in another namespace, and
not to `metrics-store`, which has no Service record) and that `pudl doctor`
reports Datalog rules ok with both packs installed.
