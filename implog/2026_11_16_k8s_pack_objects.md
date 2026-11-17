# Kubernetes pack: Services, Ingresses, Namespaces, Nodes (2026-11-16)

`pudl-3q5.3`. The second object slice of the Kubernetes pack
(`docs/design/2026-11-05-kubernetes-pack.md`, Objects), on the workload
slice's collector shapes (`implog/2026_11_16_k8s_pack_workloads.md`).
`pudl query k8s_ingress_backend` now lists, per cluster, the service and
port each Ingress host and path goes to; `pudl query k8s_node` gives each
node's `provider_id` (the VM it runs on, for `pudl-0qh`).

## Schemas

`packs/k8s/schema/k8s/network.cue`: `k8s.#Service` (`apiVersion: "v1"`,
`kind: "Service"`) and `k8s.#Ingress` (`networking.k8s.io/v1`). Identity
`["cluster", "metadata.namespace", "metadata.name"]`.

`packs/k8s/schema/k8s/nodes.cue`: `k8s.#Namespace` and `k8s.#Node`
(`v1`). Identity `["cluster", "metadata.name"]`.

All four (draft): `same_resource: [{kind: "k8s.object", key: uid:
"metadata.uid"}]`; `sensitive_fields` the last-applied annotation; both
`apiVersion` and `kind` carry `@pudl(match)` (the GCP pack's Cloud Run
`Service` has the same kind).

## Facts and rule

| Relation | Arguments |
| --- | --- |
| `k8s_service` | `cluster`, `namespace`, `name`, `type` (default `ClusterIP`), `cluster_ip` (default `""`; `None` when headless) |
| `k8s_service_port` | per `spec.ports[*]`: `name`, `port`, `protocol` (default `TCP`), `target_port`; join `k8s_service` on `resource_id` |
| `k8s_ingress` | `cluster`, `namespace`, `name`, `class` (`spec.ingressClassName`, default `""`) |
| `k8s_ingress_host` | `cluster`, `namespace`, `name`, `host`: one per rule host |
| `k8s_ingress_path` | per path of a rule: `host`, `path`, `service`, `port`; join on `resource_id` |
| `k8s_ingress_backend` (rule) | `cluster`, `namespace`, `name`, `host`, `path`, `service`, `port` |
| `k8s_namespace` | `cluster`, `name` |
| `k8s_node` | `cluster`, `name`, `provider_id` (`spec.providerID`, default `""`) |

A fact's `each` row cannot read fields outside its row, so the Ingress
shape sets `host` (the rule's host, `""` for none) and `port` (the service
backend's port number, else its name) on every path, as the Argo CD pack
sets `sourceIndex` on Helm parameters. The rule `k8s_ingress_backend`
(`packs/k8s/schema/pudl/rules/k8s_pack.cue`, the pack's first rules file)
joins `k8s_ingress_path` with `k8s_ingress` on `resource_id`.

## Collectors (`packs/k8s/schema/models/k8s_objects.cue`)

| Model | Reads |
| --- | --- |
| `k8s-services`, `k8s-ingresses` | `kubectl get services` / `ingresses.networking.k8s.io -A -o json`; `complete: false` |
| `k8s-namespaces`, `k8s-nodes` | `kubectl get namespaces` / `nodes -o json`; `complete: false` |
| `k8s-fixture-services`, `-ingresses`, `-namespaces`, `-nodes` | `populators/k8s/<kind>.json` and `<kind>-hostile.json`; `complete: true` |

Shared shapes (package `models`):

- `_k8sGet` gains `_namespaced: *true | bool`; false leaves out `-A`.
- `_k8sLive` and `_k8sFixture` (in `k8s_workloads.cue`): the live and
  fixture collectors of one kind with a `_transform`.
  `_k8sWorkloadLive` and `_k8sWorkloadFixture` are now these with
  `_k8sWorkloadTransform`.
- `_k8sURL`: a value without the userinfo of a URL with a scheme (mirrors
  the Argo CD pack's `_argocdRepoURL`, without its ssh user exception).
- `_k8sCredentialKey`: `(?i)(auth|secret|token|password|passwd|credential|key)`.
- `_k8sAnnotations`: keep every key; a credential-named key's value becomes
  `[REDACTED]`, any other value goes through `_k8sURL`. Applied to Service
  and Ingress metadata.
- `_k8sMetadata`: metadata without `managedFields`, annotations optionally
  through `_k8sAnnotations`.
- `_k8sService`, `_k8sIngress` (`_k8sIngressRule`, `_k8sIngressPath`),
  `_k8sNamespace`, `_k8sNode` (status kept: `_k8sNodeStatus` = addresses,
  nodeInfo, conditions, capacity, allocatable).
- `_k8sNamespacedTransform & {_shape: ...}` skips objects in system
  namespaces; `_k8sNamespaceTransform` skips the system namespaces
  themselves; `_k8sNodeTransform` skips nothing.

## Fixtures (`packs/k8s/populators/k8s`)

On the fixture cluster `198.51.100.30`. Services for the workload fixtures
(`shop/checkout`, `shop/catalog`, headless `shop/cart-redis`,
LoadBalancer `edge/edge-proxy`, `default/kubernetes`); Ingresses
`shop/storefront` (two hosts, a named port) and `monitoring/metrics` (a
rule without a host); the namespaces of the workload fixtures; two GKE
nodes with `gce://example-project/<zone>/<name>` provider IDs. Each kind
has objects in (or that are) system namespaces. The hostile fixtures plant
`FAKE-K8S-*` in the last-applied annotation of every kind, a Service
`example.com/api-token` annotation and a `redis://` URL's userinfo, an
Ingress `permanent-redirect` URL's userinfo, nginx `auth-secret` and
`auth-url`, Node `status.images` names, `volumesAttached`, `volumesInUse`
and `config`, `managedFields`, and system namespace objects.

## Tests (`cmd/k8s_pack_objects_test.go`)

- `TestK8sPackFixtureObjects`: facts per kind with their cluster, system
  namespaces skipped, ports per service, ingress hosts and backends, node
  provider IDs, `managedFields` and `status.images` dropped.
- `TestK8sPackHostileObjectsPlantNothing`: each hostile record's
  annotations as expected; Node status keys only the kept five; no
  `FAKE-K8S` byte under the workspace.
- `TestK8sPackLiveObjects`: the live collectors against the fake `kubectl`
  (`fakeKubectl` in `k8s_pack_workloads_test.go`, now answering the four
  new `get`s); `-A` for Services and Ingresses only.

Not done: a run against a live cluster, graph display and `in_cluster`
edges (`pudl-3q5.4`), and the Node to VM link (`pudl-0qh`).
