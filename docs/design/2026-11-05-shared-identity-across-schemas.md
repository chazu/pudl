# Shared identity across schemas

Status: accepted 2026-11-05. Issue: `pudl-q4v.9`. Follows
[GCP collectors on Cloud Asset Inventory §8](2026-11-03-gcp-cloud-asset-collectors.md#8-kubernetes-objects-and-shared-identity).

## Problem

One real resource can reach pudl through several schemas:

- A GKE Deployment through Cloud Asset Inventory (GCP pack) and through
  kubectl (a future Kubernetes pack).
- A GKE cluster as `gcp.#GKECluster` and as the Kubernetes pack's cluster
  record.
- Later: an EKS cluster, an EC2 instance that is also a Kubernetes node,
  Terraform state next to the live resource.

Each record gets its own `resource_id`, namespaced by its schema family
(`internal/identity/resource_id.go`). Nothing says the records describe one
thing, and graph export leaves a reference that matches two resources
unresolved (`refIndex.resolve` in `internal/graphexport/edges.go`).

Sources of *one* schema already reconcile: CAI and gcloud records are
normalized to one shape and share one identity. This design covers records of
*different* schemas.

## Decisions (2026-11-05, with the user)

1. **Link, don't merge.** Each representation keeps its record,
   `resource_id`, versions and facts. pudl records that they are one resource.
2. **One node in pudl ui.** A linked group is drawn as one node listing each
   representation; edges to any member attach to that node.
3. **Clusters first, then objects.** A cluster is itself a resource with
   several representations, linked by its API server endpoint. Kubernetes
   objects match on linked cluster, namespace, group, kind and name. This
   covers GKE, EKS, AKS and local clusters alike.
4. **Keys from schema paths and from rules.** A schema declares key paths for
   values its record holds; datalog rules link resources when the key needs a
   join.
5. **General mechanism.** Nothing in the core is Kubernetes- or GCP-specific.

## Design

### 1. Identity kinds

A kind names a space of real resources, independent of provider:
`k8s.cluster`, `k8s.object`. Kinds are ground facts, like the graph
vocabulary:

```cue
same_resource_kind_k8s_cluster: {
	head: {rel: "same_resource_kind", args: {kind: "k8s.cluster",
		label: "Kubernetes cluster", meaning: "A cluster, keyed by its API server host."}}
}
```

Built-ins declare cross-provider kinds (`k8s.*`); packs declare their own.
`pudl doctor` reports a schema or rule that names an undeclared kind.

### 2. Keys a record holds: `_pudl.same_resource`

```cue
_pudl: same_resource: [
	{kind: "k8s.cluster", key: {host: "endpoint"}},
	{kind: "k8s.cluster", key: {host: "privateClusterConfig.privateEndpoint"}},
]
```

- A list, so a schema can belong to several kinds, or hold several keys for
  one kind (a GKE cluster has public, private and DNS endpoints).
- Each key value is a path or `{path, trim_prefix?, last_segment?, prepend?}`,
  exactly as in `_pudl.graph.refs`. One `[*]` fans out, as in `_pudl.facts`.
- A missing path yields no key, never an empty one, so absent data never links.
- Inherited along `base_schema`; the nearest declaration wins.

Projection writes one fact per key:
`same_resource_key(kind, key, resource_id)`, where `key` is the canonical
JSON of the key map (`identity.CanonicalIdentityJSON`). Like every projected
fact it describes the resource's latest observation and is invalidated when
the resource changes or is deleted, so links follow time travel.

### 3. Links: the `same_resource` relation

`same_resource(a, b, kind)`: resources `a` and `b` are representations of one
resource of `kind`. Two ways to derive it:

- **From keys**, by a built-in rule:

  ```cue
  same_resource_by_key: {
  	head: {rel: "same_resource", args: {a: "$A", b: "$B", kind: "$K"}}
  	body: [
  		{rel: "same_resource_key", args: {kind: "$K", key: "$V", resource_id: "$A"}},
  		{rel: "same_resource_key", args: {kind: "$K", key: "$V", resource_id: "$B"}},
  	]
  }
  ```

  It also yields `a = b` rows (datalog has no variable inequality); consumers
  drop them.
- **From pack rules** that derive `same_resource` directly when the key needs a
  join. Rules may read `same_resource` itself (recursion is supported); that is
  how objects build on linked clusters (§4).

`same_resource` and `same_resource_key` are reserved
(`database.IsReservedRelation`): `AddFact` rejects them, as it does
`catalog_entry`.

**Groups.** Links are symmetric and transitive. A new package
`internal/sameresource` reads `same_resource` rows and returns connected
components; a group's ID is its smallest member `resource_id`, so it changes
only when membership does. Groups are computed in Go, not datalog, because
conflicts (below) must be removed before closure.

**Conflicts.** A group holds at most one resource per identity namespace (the
schema family root `ComputeResourceID` uses). Two resources of one namespace
with the same key are not two views of one thing: a collision, or a stale
record. A component that would hold two is not formed; its members stay
standalone and `pudl doctor` reports the component with its kind and key.
A wrong link is worse than a missing one.

### 4. Kubernetes, worked through

**Clusters** (`k8s.cluster`, key `host`):

| Representation | Key source |
| --- | --- |
| `gcp.#GKECluster` | `_pudl.same_resource` on `endpoint`, `privateClusterConfig.privateEndpoint` and the DNS endpoint |
| `aws.#EKSCluster` (later) | `endpoint` with `trim_prefix: "https://"` |
| Kubernetes pack cluster record, one per kube context | the context's server URL, normalized to its host by the collector's transform (scheme and `:443` removed) |

**Objects** (`k8s.object`):

- kubectl collectors stamp each object with its context's `cluster_host`
  through the run's `set`, and project
  `k8s_object(resource_id, cluster_host, namespace, group, kind, name)` and
  `k8s_cluster(resource_id, host)`.
- The GCP pack projects
  `gcp_k8s_object(resource_id, cluster, namespace, group, kind, name)` from CAI,
  where `cluster` is the cluster's path from the asset name.
- A bridge rule in the GCP pack links them through the linked clusters:

```cue
gcp_same_k8s_object: {
	head: {rel: "same_resource", args: {a: "$A", b: "$B", kind: "k8s.object"}}
	body: [
		{rel: "gcp_k8s_object", args: {resource_id: "$A", cluster: "$CL", namespace: "$NS", group: "$G", kind: "$K", name: "$N"}},
		{rel: "gcp_gke_cluster", args: {resource_id: "$GC", self_link: "$CL"}},
		{rel: "same_resource", args: {a: "$GC", b: "$KC", kind: "k8s.cluster"}},
		{rel: "k8s_cluster", args: {resource_id: "$KC", host: "$H"}},
		{rel: "k8s_object", args: {resource_id: "$B", cluster_host: "$H", namespace: "$NS", group: "$G", kind: "$K", name: "$N"}},
	]
}
```

Bridge rules live in the pack of the platform that hosts the clusters (GKE in
the GCP pack, EKS in the AWS pack). They name the Kubernetes pack's relations,
so `k8s_cluster` and `k8s_object` become a documented contract of that pack.
Without the Kubernetes pack those relations have no facts and the rule
matches nothing; the datalog engine does not reject unknown relations.

### 5. Consumers

- **Graph export.** Groups are computed before nodes are built. A group is one
  `Resource` with the group ID, carrying `representations: [{id, schema,
  origin, type}]`; its type and display come from the member whose schema name
  sorts first. `refIndex` maps every member's ID and refs to the group, and a
  reference matching several members of one group resolves to it. Edges from
  members point at the group; duplicate edges merge their evidence as today.
- **pudl ui.** The inspector lists a node's representations.
- **`pudl show ID`.** A "Same resource as" section: each other member's ID,
  schema and kind (`same_resource` in `--json`).
- **`pudl query same_resource`.** Usable in rules and checks like any relation.
- **`pudl doctor`.** Undeclared kinds and conflicting components.

### 6. Not in scope

- Merging records or versions across representations.
- De-duplicating check findings across a group's members.
- Fuzzy matching. Links come only from declared keys and rules.

## Delivery

| Step | Work | Estimate |
| --- | --- | --- |
| 1 | Core: `same_resource_kind` facts, `_pudl.same_resource` (parse, validate, inherit), projection to `same_resource_key`, built-in rule, reserved relations, `internal/sameresource` groups and conflicts | about 1 day |
| 2 | Consumers: graph export groups and ref resolution, `pudl show`, doctor checks, docs (schema authoring, datalog, pudl-core skill) | about 1 day |
| 3 | pudl ui inspector: representations | about half a day |
| 4 | GCP pack: `k8s.cluster` keys on `gcp.#GKECluster` after a live check of the endpoint fields; CAI Kubernetes schemas with `gcp_k8s_object`; bridge rule | about 1 day |
| 5 | Kubernetes pack (its own epic): cluster record, object schemas, collectors stamping `cluster_host` | separate |

Steps 1 to 3 are tested with synthetic fixtures and need no Kubernetes pack.

## Open questions

1. Which GKE field holds the DNS-based control plane endpoint, and is it in
   CAI's copy? Live check in step 4.
2. Display precedence inside a group: first schema by name to start; a
   declared preference per kind if that reads badly.
