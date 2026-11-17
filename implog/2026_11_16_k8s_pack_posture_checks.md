# Kubernetes pack: container posture check family (2026-11-16)

`pudl-5zh`. The checks follow-up of the Kubernetes pack
(`docs/design/2026-11-05-kubernetes-pack.md`, Delivery). A checks-only
family over the workload collectors, built like the GCP pack's families
(`packs/gcp/schema/models/gcp_pack_checks.cue`).

## Facts (`packs/k8s/schema/k8s/workloads.cue`)

`k8s.#Deployment`, `k8s.#StatefulSet` and `k8s.#DaemonSet` project three
shared relations (`_k8sPodFacts`), keyed by the record's `resource_id`:

- `k8s_container` (each `spec.template.spec.containers[*]`) and
  `k8s_init_container` (each `...initContainers[*]`), with the args
  `_k8sContainerArgs`: `container`; `image_pin`, a rewrite of `image` to
  `digest` (contains `@`), `latest` (ends `:latest`), `tag` (ends
  `:<tag>` after the last `/`, so a registry port is not a tag) or
  `untagged`; `privileged` (default false); `sets_run_as_non_root`,
  `run_as_non_root` (default false); `sets_run_as_user`, `run_as_user`
  (default 0); `has_cpu_limit`, `has_memory_limit` (`exists` on
  `resources.limits.cpu` / `.memory`).
- `k8s_pod_security`: the pod's `sets_run_as_user`, `run_as_user`,
  `run_as_non_root`.

Datalog has no negation, so "not set" is an `exists` arg next to a
defaulted value. Nothing reads env, command or args; the stored records
are unchanged.

## Rules (`packs/k8s/schema/pudl/rules/k8s_posture.cue`)

- `k8s_workload(resource_id, cluster, namespace, kind, name)`: one rule
  per kind.
- `k8s_pod_container`: `k8s_container` / `k8s_init_container` with its
  workload and `list` (`containers` / `initContainers`).
- `k8s_privileged_container`: `privileged: true`.
- `k8s_unpinned_image`: `image_pin` `latest` or `untagged`.
- `k8s_container_without_limits`: three rules; `missing` is `cpu`,
  `memory` or `cpu and memory`.
- `k8s_container_may_run_as_root`: four rules, with the container's
  `securityContext` overriding the pod's per field: container
  `runAsUser: 0`; pod `runAsUser: 0` and none on the container; no
  `runAsUser` and container `runAsNonRoot: false`; no `runAsUser` and no
  `runAsNonRoot: true` on either.

Every check row has `resource_id`, `cluster`, `namespace`, `kind`, `name`,
`container`, `list` (plus `missing` for limits). `resource_id` follows
from the identity (cluster, namespace, name), so finding keys stay stable
and each finding resolves to its workload record. `list` is a string:
a boolean head constant comes back from SQLite as 0 or 1.

The task text says a pod's `runAsNonRoot: true` is enough when the
container does not set it to true. The rules use Kubernetes precedence: a
container's explicit `runAsNonRoot: false` overrides the pod's `true` and
fails (cart-redis's `redis` in the fixtures).

## Family (`packs/k8s/schema/models/k8s_pack_checks.cue`)

`k8s-posture-checks` and `k8s-fixture-posture-checks` (`_#K8sPosture`
with `_prefix`): `depends_on` the three workload collectors; every check
has evidence `model:<prefix><kind>` for all three, `max_age: "1h"`,
`expect: "empty"`.

| Check | Query | Severity |
| --- | --- | --- |
| `privileged-container` | `k8s_privileged_container` | fail |
| `unpinned-image` | `k8s_unpinned_image` | warn |
| `no-resource-limits` | `k8s_container_without_limits` | warn |
| `may-run-as-root` | `k8s_container_may_run_as_root` | warn |

The live collectors declare `complete: false`, so the live family is
`unknown` (`incomplete_observation`) until `pudl-u29` scopes observations
per cluster. This is the evidence rule applied as designed, not a defect
of the family.

## Fixtures (`packs/k8s/populators/k8s/`)

No workload added or removed; the hostile fixtures are unchanged.

| Workload | Container | Fails |
| --- | --- | --- |
| Deployment shop/checkout (pod `runAsNonRoot: true`) | checkout: tag, both limits | none |
| | proxy: tag and digest, `privileged: false`, `runAsUser: 0`, no limits | limits (both), root |
| | migrate (init): digest only, memory limit only | limits (cpu) |
| Deployment shop/catalog | catalog: `:latest`, `runAsUser: 10001`, cpu limit only | image, limits (memory) |
| Deployment edge/edge-proxy (pod `runAsUser: 0`) | proxy: `:latest@sha256:…`, both limits | root |
| StatefulSet shop/cart-redis (pod `runAsNonRoot: true`) | redis: `runAsNonRoot: false`, both limits | root |
| | init-config (init, new): `registry.example.com:5000/tools/redis-init`, both limits | image |
| StatefulSet monitoring/metrics-store (pod `runAsUser: 65534`) | store: no tag, no limits | image, limits (both) |
| DaemonSet logging/log-agent | agent: `privileged: true`, no limits | privileged, limits (both), root |

The six hostile containers fail limits (both) and root.

## Tests (`cmd/k8s_pack_checks_test.go`)

- `TestK8sPackPostureChecks`: fixture collectors and the family; each
  check's exact findings (1, 3, 11, 10), severities, a `resource_id` on the
  witness, no planted value stored; then a waiver file for log-agent's
  privileged container makes that check `waived` and the command exit 0.
- `TestK8sPackPostureChecksUnknown`: each family alone is `unknown`
  (`missing_snapshot`); the live family after the live collectors (fake
  kubectl) is `unknown` (`incomplete_observation`).
- `useFakeKubectl` (in `cmd/k8s_pack_workloads_test.go`) now sets up the
  fake kubectl for both live tests.

## Public API

No Go API change. Pack surface: models `k8s-posture-checks`,
`k8s-fixture-posture-checks`; facts `k8s_container`, `k8s_init_container`,
`k8s_pod_security`; rules `k8s_workload`, `k8s_pod_container`,
`k8s_privileged_container`, `k8s_unpinned_image`,
`k8s_container_without_limits`, `k8s_container_may_run_as_root`.
