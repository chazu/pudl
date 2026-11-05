# GCP pack: GKE cluster keys and Kubernetes deployments (2026-11-05)

Issue: `pudl-h7s.4` (epic `pudl-h7s`, now complete). Design:
`docs/design/2026-11-05-shared-identity-across-schemas.md` §4, amended with
the live check (see its "Findings during delivery").

## Live check (field names only)

The organization's GKE clusters; CAI and gcloud identical. Unique per
cluster: `privateClusterConfig.publicEndpoint` and
`controlPlaneEndpointsConfig.dnsEndpointConfig.endpoint`. Private
endpoints repeat; `endpoint` is the private IP on private clusters. CAI
exports Kubernetes objects with a distinct `metadata.uid`, no
`apiVersion`/`kind`; many Deployments are in system namespaces, and most
carry env values.

Decision amended with the user: Kubernetes objects key on `metadata.uid`
("whatever works best for other types of Kubernetes clusters"), not a bridge
rule through linked clusters.

## Public API

### Schemas

- `gcp.#GKECluster`: `same_resource` kind `k8s.cluster`, key `host` from
  `privateClusterConfig.publicEndpoint` and the DNS endpoint. Fact
  `gcp_gke_cluster` gains `location`.
- `gcp.#K8sDeployment` (new, `packs/gcp/schema/gcp/k8s.cue`): resource type
  `gcp.k8s.deployment`, identity `assetName` (`@pudl(match)` on the CAI asset
  name), fields `project`, `location`, `cluster`, `apiVersion`, `kind`,
  `metadata`, `spec`, `status`. `same_resource` kind `k8s.object`, key `uid`
  from `metadata.uid`. Fact `gcp_k8s_deployment(project, location, cluster,
  namespace, name, replicas)`. Redacts container and init container env
  values and the last-applied annotation. Owner from `metadata.labels.team`.

### Collectors

- `gcp-k8s-deployments` / `gcp-fixture-k8s-deployments`: CAI search for
  `apps.k8s.io/Deployment`. `_gcpCAIOnly` marks types gcloud cannot list:
  they search CAI whatever `_gcpAssetScope` says and have no fallback.
- `_gcpK8sShape`: restores `apiVersion`/`kind`, records `assetName`,
  `location` and `cluster` from the asset name, drops `managedFields`. The
  CAI transform now passes the asset to shapes as `_asset`.
- `_gcpSkip` drops objects in `kube-system`, `kube-public`,
  `kube-node-lease`, `gke-*` and `gmp-*` namespaces.
- Fixture: `populators/gcp/cai/k8s-deployments.json` (synthetic; one
  system-namespace deployment that is skipped).

### Graph

- Built-in edge type `in_cluster` (containment).
- Rule `gcp_graph_k8s_deployment_cluster`: deployment → GKE cluster, joined
  on project, location and name (a zonal cluster's selfLink says `zones/`).

### Fix in the shared identity core

- `database.SnapshotEvidenceSource`, `IsSnapshotEvidenceSource`,
  `SnapshotEvidenceSourcePrefix`. Snapshot-scoped checks re-project records
  into an evidence view under `snapshot:<id>`; `same_resource_key` is now
  allowed there. Before, the GKE posture checks could not reach a verdict.
  `internal/evidence/view.go` uses the helper.

### Docs

- `packs/gcp/README.md` (collector, redaction, CAI-only note, `in_cluster`,
  "Same resource from other sources"); design doc amended;
  `docs/schema-authoring.md`, `docs/datalog.md` and the pudl-core skill
  examples corrected (unique keys only; objects by UID).

## Tests

- `cmd/graph_export_test.go`: both fixture deployments have `in_cluster`
  edges to `prod-private`; the kube-system one is not collected.
- `cmd/gcp_pack_*_test.go`: `k8s-deployments` in the pack's type lists.
- `internal/database/same_resource_relations_test.go`: evidence view keys
  accepted.
- Smoke (temp workspace): `same_resource_key` facts for the cluster's public
  endpoint and both deployments' UIDs; env values and the last-applied
  annotation stored as `[REDACTED]`; `managedFields` absent; `kind` restored.
- `CGO_ENABLED=0 go test ./...` passes; `make lint` 0 issues.
