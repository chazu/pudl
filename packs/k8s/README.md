# Kubernetes content pack

An installable set of CUE files for cataloging Kubernetes clusters with PUDL
through `kubectl`, for any distribution: a draft cluster schema, a live
collector that reads kubectl's current context, an offline fixture collector,
and the run shapes other kubectl collectors build on. It contains no
vendor-specific Go. The design is
`docs/design/2026-11-05-kubernetes-pack.md` in the PUDL source.

```bash
pudl init
pudl pack install gcp   # optional: links the cluster to its GKE record
pudl pack install k8s
pudl check k8s-fixture-cluster   # fixture; no cluster access
pudl check k8s-cluster           # the cluster of kubectl's current context
```

Installed files, under `.pudl/`:

| Path | Contents |
| --- | --- |
| `schema/k8s/cluster.cue` | `k8s.#Cluster`: the cluster a kube context reaches (draft) |
| `schema/models/k8s_pack.cue` | The collectors, and the shared run shapes below |
| `populators/k8s/cluster.sh` | Helper `k8s-cluster` runs: the current context's cluster as one object |
| `populators/k8s/cluster.json` | Fixture: what the helper prints for a context on the GCP pack's fixture cluster `prod-private` |

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

## Facts

| Relation | Arguments |
| --- | --- |
| `k8s_cluster` (fact) | `host`, `context`, `cluster`, `server`, `server_version` |

```bash
pudl query k8s_cluster
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
