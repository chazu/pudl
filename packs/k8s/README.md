# Kubernetes content pack

An installable set of CUE files for cataloging Kubernetes clusters with PUDL
through `kubectl`, for any distribution: draft schemas for the cluster and
its workloads (Deployments, StatefulSets, DaemonSets), live collectors that
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
```

Installed files, under `.pudl/`:

| Path | Contents |
| --- | --- |
| `schema/k8s/cluster.cue` | `k8s.#Cluster`: the cluster a kube context reaches (draft) |
| `schema/k8s/workloads.cue` | `k8s.#Deployment`, `k8s.#StatefulSet`, `k8s.#DaemonSet` (draft) |
| `schema/models/k8s_pack.cue` | The cluster collectors, and the shared run shapes below |
| `schema/models/k8s_workloads.cue` | The workload collectors and their shape |
| `populators/k8s/cluster.sh` | Helper `k8s-cluster` runs: the current context's cluster as one object |
| `populators/k8s/cluster.json` | Fixture: what the helper prints for a context on the GCP pack's fixture cluster `prod-private` |
| `populators/k8s/<kind>.json` | Fixtures: `kubectl get <kind> -A -o json` on that cluster (`deployments`, `statefulsets`, `daemonsets`) |
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

## Facts

| Relation | Arguments |
| --- | --- |
| `k8s_cluster` (fact) | `host`, `context`, `cluster`, `server`, `server_version` |
| `k8s_deployment` (fact) | `cluster`, `namespace`, `name`, `replicas` (default 1) |
| `k8s_statefulset` (fact) | `cluster`, `namespace`, `name`, `replicas` (default 1) |
| `k8s_daemonset` (fact) | `cluster`, `namespace`, `name` |

`cluster` on a workload fact is the `host` of a `k8s_cluster` fact.

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
  unwrap.

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
