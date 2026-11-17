# Kubernetes content pack

An installable set of CUE files for cataloging Kubernetes clusters with PUDL
through `kubectl`, for any distribution: draft schemas for the cluster and
its workloads (Deployments, StatefulSets, DaemonSets), Services, Ingresses,
Namespaces and Nodes, live collectors that
read kubectl's current context, offline fixture collectors, and the run
shapes other kubectl collectors build on. It contains no
vendor-specific Go. The design is
`docs/design/2026-11-05-kubernetes-pack.md` in the PUDL source.

```bash
pudl init
pudl pack install gcp   # optional: links the cluster to its GKE record
pudl pack install k8s
pudl check k8s-fixture-cluster   # fixture; no cluster access
pudl check k8s-cluster           # the cluster of kubectl's current context
pudl check k8s-deployments k8s-statefulsets k8s-daemonsets
pudl query k8s_statefulset       # every StatefulSet, with its cluster
pudl check k8s-services k8s-ingresses k8s-namespaces k8s-nodes
pudl query k8s_ingress_backend   # the service and port of every Ingress host and path
pudl graph export                # every object in its cluster
```

Installed files, under `.pudl/`:

| Path | Contents |
| --- | --- |
| `schema/k8s/cluster.cue` | `k8s.#Cluster`: the cluster a kube context reaches (draft) |
| `schema/k8s/workloads.cue` | `k8s.#Deployment`, `k8s.#StatefulSet`, `k8s.#DaemonSet` (draft) |
| `schema/k8s/network.cue` | `k8s.#Service`, `k8s.#Ingress` (draft) |
| `schema/k8s/nodes.cue` | `k8s.#Namespace`, `k8s.#Node` (draft) |
| `schema/models/k8s_pack.cue` | The cluster collectors, and the shared run shapes below |
| `schema/models/k8s_workloads.cue` | The workload collectors and their shape; `_k8sLive`, `_k8sFixture` |
| `schema/models/k8s_objects.cue` | The Service, Ingress, Namespace and Node collectors and their shapes |
| `schema/pudl/rules/k8s_pack.cue` | The rule `k8s_ingress_backend` |
| `schema/pudl/rules/k8s_graph.cue` | Graph edges: `in_cluster`, `routes_to` |
| `populators/k8s/cluster.sh` | Helper `k8s-cluster` runs: the current context's cluster as one object |
| `populators/k8s/cluster.json` | Fixture: what the helper prints for a context on the GCP pack's fixture cluster `prod-private` |
| `populators/k8s/<kind>.json` | Fixtures: `kubectl get <kind> -o json` on that cluster (`deployments`, `statefulsets`, `daemonsets`, `services`, `ingresses`, `namespaces`, `nodes`) |
| `populators/k8s/<kind>-hostile.json` | Fixtures that plant values the shape and redaction must drop |

## The live collectors

Every live collector reads kubectl's **current context** only. To collect
another cluster, switch context and run again. A collector's observation
scope is fixed while the context is not, so the live collectors declare
`complete: false`: a run never marks the previous cluster's records gone.
Records of a cluster you no longer collect stay current until you delete
them by hand.

The collectors run only read-only kubectl commands: `config view`, `version`
and `get`.

## `k8s.#Cluster`

One record per run: the cluster of the current context. Identity: `host`.

| Field | From |
| --- | --- |
| `context` | The context's name (`kubectl config view --minify`, `contexts[0].name`) |
| `cluster` | The kubeconfig cluster entry's name (`clusters[0].name`) |
| `server` | The API server URL (`clusters[0].cluster.server`) |
| `host` | `server` with its scheme, a `:443` port and a trailing slash removed |
| `serverVersion` | `kubectl version -o json`, `serverVersion.gitVersion` |

The helper `populators/k8s/cluster.sh` prints these as one JSON object: the
context and cluster entries of the minified kubeconfig, the `kubectl version`
output and `host`. It prints no credentials: `kubectl config view` omits
certificate data and tokens, and the helper does not print the user entry.
The transform keeps only the fields above. A server that cannot be reached
fails the run (`kubectl version` exits 1), so nothing is stored for it.

Two commands are needed, so the record is composed by a helper rather than
printed by one `kubectl` invocation: no single command prints both the
server URL and the server version.

### Links to the cloud provider's record

The record declares `_pudl.same_resource` kind `k8s.cluster`, key `host`.
The GCP pack's `gcp.#GKECluster` declares the same kind from the cluster's
public endpoint and its DNS endpoint, so a context that reaches a GKE
cluster by either is one group with the GKE record: one node in `pudl ui`,
"Same resource as" in `pudl show`. A context that reaches a cluster by its
private endpoint does not link: private endpoints repeat across VPCs.

```bash
pudl check gcp-fixture-gke-clusters k8s-fixture-cluster
pudl list --schema 'k8s.#Cluster'
pudl show <entry>     # Same resource as (k8s.cluster): ... gcp.#GKECluster
```

## Workloads: `k8s.#Deployment`, `k8s.#StatefulSet`, `k8s.#DaemonSet`

One collector per kind (`k8s-deployments`, `k8s-statefulsets`,
`k8s-daemonsets`). Each enumerates the current cluster's host
(`_k8sClusterEach`), runs `kubectl get <kind>.apps -A -o json` and stores one
record per object, with `cluster` set to the host: the identity of the
cluster record.

- **Identity:** `cluster`, `metadata.namespace`, `metadata.name`. These stay
  when an object is recreated; the UID does not.
- **Shared identity:** `_pudl.same_resource` kind `k8s.object`, key
  `metadata.uid`. The GCP pack's `gcp.#K8sDeployment` declares the same, so
  a Deployment seen by both packs is one group.
- **Skipped:** objects in `kube-system`, `kube-public`, `kube-node-lease`,
  `gke-*` and `gmp-*`.
- **Shape** (`_k8sWorkload` in `k8s_workloads.cue`): the object as kubectl
  prints it, without `metadata.managedFields`; every pod template
  annotation keeps its key and its value becomes `[REDACTED]`. Agents and
  injectors read their configuration, credentials included, from these
  annotations.
- **Redacted** by the schemas before anything is stored: the `value` of
  every container and init container `env` entry, every element of their
  `command` and `args` (each becomes `[REDACTED]`, so the count stays;
  flags and connection strings there often carry credentials), and the
  `kubectl.kubernetes.io/last-applied-configuration` annotation, which
  repeats the object as applied. `valueFrom` holds references only (a
  Secret or ConfigMap name and key) and is kept, as are `image`, `name`,
  `ports`, `resources` and `volumeMounts`.

## Services, Ingresses, Namespaces and Nodes

One collector per kind (`k8s-services`, `k8s-ingresses`, `k8s-namespaces`,
`k8s-nodes`), as for workloads. Services and Ingresses
(`ingresses.networking.k8s.io`) are listed with `-A`; Namespaces and Nodes
are cluster-scoped and listed without it.

- **Identity:** `cluster`, `metadata.namespace`, `metadata.name` for
  Services and Ingresses; `cluster`, `metadata.name` for Namespaces and
  Nodes.
- **Shared identity:** `k8s.object`, key `metadata.uid`.
- **Skipped:** Services and Ingresses in the system namespaces, and the
  system namespaces themselves (no Namespace record for `kube-system`,
  `gke-*` and the rest).
- **Shape** (`k8s_objects.cue`): the object without
  `metadata.managedFields`. A Node keeps only `addresses`, `nodeInfo`,
  `conditions`, `capacity` and `allocatable` of its `status`; `images` (every
  image the node pulled), volumes, daemon endpoints and config are
  dropped. Every path of an Ingress rule gets `host` (the rule's host, `""`
  for none) and `port` (its service backend's port number, else its name),
  for the `k8s_ingress_path` fact.
- **Annotations** of Services and Ingresses keep every key. The value of
  one whose key matches `(?i)(auth|secret|token|password|passwd|credential|key)`
  (nginx's `auth-url` and `auth-secret`, for example) becomes `[REDACTED]`;
  any other value that is a URL with a scheme loses its userinfo
  (`https://user:TOKEN@host/x` becomes `https://host/x`). Namespace and Node
  annotations are kept.
- **Redacted** by the schemas: the
  `kubectl.kubernetes.io/last-applied-configuration` annotation on every
  kind.

## Graph

`pudl graph export` and `pudl ui` draw every type with a code and a class
(`_pudl.graph.display`), named by `metadata.name` (the cluster by `host`):

| Type | Code | Label | Class |
| --- | --- | --- | --- |
| `k8s.cluster` | `K8S` | Kubernetes cluster | compute |
| `k8s.deployment` | `DEP` | Kubernetes deployment | compute |
| `k8s.statefulset` | `STS` | Kubernetes stateful set | compute |
| `k8s.daemonset` | `DS` | Kubernetes daemon set | compute |
| `k8s.service` | `SVC` | Kubernetes service | edge |
| `k8s.ingress` | `ING` | Kubernetes ingress | edge |
| `k8s.namespace` | `NS` | Kubernetes namespace | admin |
| `k8s.node` | `NODE` | Kubernetes node | compute |

Edges (`schema/pudl/rules/k8s_graph.cue`), basis `observed`, both built-in
edge types:

- `in_cluster`, from every Deployment, StatefulSet, DaemonSet, Service,
  Ingress, Namespace and Node to the `k8s.#Cluster` record whose `host` is
  the object's `cluster`. An object whose cluster has no record has no edge.
- `routes_to`, from an Ingress to each Service in its namespace that one of
  its paths names (`k8s_ingress_backend`). A Service with no record, and a
  path with a resource backend, give no edge.

Linked records are one node. With the GCP pack, the GKE cluster whose public
or DNS endpoint is the context's host and the `k8s.#Cluster` record are one
node (`k8s.cluster`); a Deployment that GCP and kubectl both see is one node
(`k8s.object`). The GCP pack's `in_cluster` edge from its deployment record
to the GKE record and this pack's edge then are the same edge.
`cmd/k8s_pack_graph_test.go` checks this with both packs' fixtures.

Not drawn: a Namespace does not contain its objects (pudl has no built-in
`in_namespace` edge type), and Nodes do not link to their VMs yet
(`pudl-0qh`).

## Facts

| Relation | Arguments |
| --- | --- |
| `k8s_cluster` (fact) | `host`, `context`, `cluster`, `server`, `server_version` |
| `k8s_deployment` (fact) | `cluster`, `namespace`, `name`, `replicas` (default 1) |
| `k8s_statefulset` (fact) | `cluster`, `namespace`, `name`, `replicas` (default 1) |
| `k8s_daemonset` (fact) | `cluster`, `namespace`, `name` |
| `k8s_service` (fact) | `cluster`, `namespace`, `name`, `type` (default `ClusterIP`), `cluster_ip` (default `""`) |
| `k8s_service_port` (fact) | one per port: `name`, `port`, `protocol` (default `TCP`), `target_port`; join `k8s_service` on `resource_id` |
| `k8s_ingress` (fact) | `cluster`, `namespace`, `name`, `class` (`spec.ingressClassName`, default `""`) |
| `k8s_ingress_host` (fact) | `cluster`, `namespace`, `name`, `host`: one per rule host |
| `k8s_ingress_path` (fact) | one per path of a rule: `host`, `path`, `service`, `port`; join on `resource_id` |
| `k8s_ingress_backend` (rule) | `cluster`, `namespace`, `name`, `host`, `path`, `service`, `port`: `k8s_ingress_path` with its Ingress |
| `k8s_namespace` (fact) | `cluster`, `name` |
| `k8s_node` (fact) | `cluster`, `name`, `provider_id` (`spec.providerID`, e.g. `gce://project/zone/name`; default `""`) |

`cluster` on an object fact is the `host` of a `k8s_cluster` fact.

```bash
pudl query k8s_cluster
pudl query k8s_statefulset cluster=198.51.100.30
```

## Shared run shapes

`schema/models/k8s_pack.cue` is in package `models`, which every installed
pack shares, so other kubectl collectors, in this pack or in another (the
Argo CD pack), reuse these:

- `_k8sClusterHelper`: the helper's argv.
- `_k8sClusterEach`: an `each` that enumerates the current cluster's `host`,
  one input. A collector sets it on every object it collects
  (`set: cluster: "{{item}}"`) to point at the cluster record.
- `_k8sListTransform`: `{in: _, out: in.items}`. `kubectl get -o json` prints
  one `List` object; a run's transform may return a list, and each element is
  stored as a record, so this unwraps the items without a JSON tool or
  `jsonpath`. Compose it with a per-item shape:
  `out: [for item in in.items {_shape & {_in: item}}]`.
- `_k8sGet & {_resource: "deployments.apps"}`: a run that lists one resource
  type across all namespaces (`kubectl get <resource> -A -o json`) with the
  unwrap. `_namespaced: false` leaves out `-A`, for a cluster-scoped type.

A fixture collector reads a saved `kubectl get -o json` List with `cat` and
the same transform.

## Fixtures

`populators/k8s/cluster.json` is what the helper prints for a context on the
GCP pack's fixture cluster `prod-private`: its `host` is that cluster's
public endpoint, so `cmd/k8s_pack_test.go` can check the link with both
packs installed. All values are placeholders.

The workload fixtures are on the same cluster: the fixture collectors set
`cluster` to its host. The Deployments `shop/checkout` and `shop/catalog`
have the UIDs of the GCP pack's fixture deployments, so they link to them;
each kind has objects in system namespaces, which are skipped. The hostile
fixtures plant `FAKE-K8S-*` values in env values, init container env
values, the last-applied annotation, pod template annotations,
`managedFields` and a system namespace; `cmd/k8s_pack_workloads_test.go`
finds none of them stored. It also runs the live collectors against a fake
`kubectl`.

The Service, Ingress, Namespace and Node fixtures are on the same cluster.
The nodes' provider IDs are `gce://example-project/<zone>/<name>`. Their
hostile fixtures plant `FAKE-K8S-*` values in the last-applied annotation
of every kind, credential-named Service and Ingress annotations, URL
userinfo in annotation values, Node `status.images` and other Node status,
`managedFields` and system namespace objects;
`cmd/k8s_pack_objects_test.go` finds none of them stored.
