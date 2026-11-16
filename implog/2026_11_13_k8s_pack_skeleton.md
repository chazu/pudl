# Kubernetes pack skeleton: layout, registry, `k8s.#Cluster` (2026-11-16)

Closes `pudl-3q5.1`. The first slice of the Kubernetes pack
(`docs/design/2026-11-05-kubernetes-pack.md`): the pack's layout and
registry entry, the cluster record and its collectors, and the run shapes
the object collectors (`pudl-3q5.2`, `pudl-3q5.3`) and the Argo CD pack
(`pudl-avqx.4`) build on.

## Core: a run transform may return a list

`kubectl get -o json` prints one `List` object with `items`. Runs decoded an
array as one record per element but could not unwrap a List, so the
design's open question was whether a helper should print the items with
`jsonpath={.items}` or a JSON tool. Neither: a command or HTTP run's
`transform` may now return a list of objects, and each element is stored as
a record. `transform: {in: _, out: in.items}` unwraps a List in CUE.

- `internal/systemmodel/transform.go`: `Transform.Records(record)
  ([]map[string]any, error)` joins `Apply`: one record for an object `out`,
  none for `null`, one per element of a list `out`. An element that is not
  an object, or any other `out`, is an error. `Apply` is unchanged and
  still rejects a list.
- `internal/populate/command.go` (`decodeCommandRecords`) and
  `http_records.go` call `Records`; `set` and the rejected-key check apply
  to every record kept.
- `internal/systemmodel/schema.cue` and `docs/observation-scope.md`
  document the list case.
- Tests: `TestTransformRecordsUnwrapsList`,
  `TestTransformRecordsRejectsNonObjectElements` (systemmodel);
  `TestRunTransformUnwrapsListItems` on a List-shaped fixture
  (`internal/populate/testdata/kubectl_list.json`) with `set` applied to
  each item, and `TestRunTransformListElementMustBeAnObject`, where a
  string item fails the run.

No cluster was reachable on 2026-11-16 (every context timed out or had
expired credentials), so the List shape comes from the kubectl
documentation; the first live object collector confirms it. The design
doc's open question 1 records the decision.

## The pack (`packs/k8s`)

| Path | Contents |
| --- | --- |
| `bundle.go` | `k8spack.Files` (embed) |
| `README.md` | Install, the collectors, the schema, links, facts, shared run shapes, fixtures |
| `schema/k8s/cluster.cue` | `k8s.#Cluster` |
| `schema/models/k8s_pack.cue` | `k8s-cluster`, `k8s-fixture-cluster`, and the shared shapes |
| `populators/k8s/cluster.sh` | Helper: the current context's cluster as one object |
| `populators/k8s/cluster.json` | Fixture: the helper's output for a context on the GCP fixture cluster `prod-private` |

Registered as `k8s` in `internal/packs/packs.go` (after `pulumi`); listed
in `docs/README.md`.

### `k8s.#Cluster`

Fields `context`, `cluster`, `server`, `host`, `serverVersion`; identity
`host`; `same_resource: [{kind: "k8s.cluster", key: host: "host"}]`;
display code `K8S`; fact `k8s_cluster(host, context, cluster, server,
server_version)`.

### The helper

`cluster.sh` prints `{"context": <contexts[0]>, "cluster": <clusters[0]>,
"version": <kubectl version -o json>, "host": ...}` from `kubectl config
view --minify -o jsonpath=...` and `kubectl version -o json`. It prints no
credentials: `config view` omits certificate data and tokens, and the user
entry is not printed. `host` is the server URL without its scheme, a `:443`
port and a trailing slash, computed by the helper so an `each` can select
it. An unreachable server fails the run (`kubectl version` exits 1). A
helper is used because no single kubectl command prints both the server
URL and the server version; the record's shape is still the CUE transform
`_k8sClusterTransform`.

### Shared shapes (package `models`, visible to other packs)

- `_k8sClusterHelper`: the helper's argv.
- `_k8sClusterEach`: `{argv: _k8sClusterHelper, select: "host"}`, the
  one-input enumeration object collectors set on every record.
- `_k8sListTransform`: `{in: _, out: in.items}`.
- `_k8sGet & {_resource: "deployments.apps"}`: `kubectl get <resource> -A
  -o json` with the unwrap.

### Tests

- `cmd/k8s_pack_test.go`, `TestK8sPackClusterSharesIdentityWithGKE`:
  installs the GCP and Kubernetes packs, runs `gcp-fixture-gke-clusters`
  and `k8s-fixture-cluster`, checks the stored record's fields, that `pudl
  show` lists the GKE cluster as the same resource (and from the GKE side,
  `prod-private` links while `legacy-public` and `batch-private` do not),
  the `k8s_cluster` fact, and `doctor`'s same-resource check.
- `internal/packs/packs_test.go`: the available-pack list.
