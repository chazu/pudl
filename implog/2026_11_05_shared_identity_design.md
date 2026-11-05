# Design: shared identity across schemas (2026-11-05)

Issue: `pudl-q4v.9` (closed). Design (accepted):
`docs/design/2026-11-05-shared-identity-across-schemas.md`.

## Decided with the user

1. Link records of different schemas that describe one resource; never merge
   them. Each keeps its record, `resource_id`, versions and facts.
2. pudl ui draws a linked group as one node listing its representations.
3. Kubernetes: clusters are linked first, by API server host; objects are
   linked through the linked clusters (namespace, group, kind, name). This works
   for any cluster, not only GKE.
4. Keys come from schema paths (`_pudl.same_resource`) and from pack rules
   that derive `same_resource` when the key needs a join.

## Planned public surface (not implemented yet)

- Schema metadata `_pudl.same_resource: [{kind, key: {name: path | {path,
  trim_prefix?, last_segment?, prepend?}}}]`.
- Relations: `same_resource_kind(kind, label, meaning)` (ground facts),
  `same_resource_key(kind, key, resource_id)` (projected, reserved),
  `same_resource(a, b, kind)` (derived, reserved).
- `internal/sameresource`: groups (connected components, ID = smallest
  member `resource_id`), conflict detection (two members of one identity
  namespace).
- Graph export `representations`, `pudl show` "Same resource as", doctor
  checks.

## Verified while designing

- Projection fans `[*]` out into one fact per element, and projected facts
  carry `entry_id` and `resource_id` implicitly (`docs/projection.md`).
- The datalog engine does not reject a body atom on a relation with no facts,
  so a GCP bridge rule can name the future Kubernetes pack's relations.
- Graph export resolves references only in Go (`refIndex` in
  `internal/graphexport/edges.go`) and leaves a reference matching two
  resources unresolved; groups fix that.

## Follow-ups filed

- Epic `pudl-h7s`: `h7s.1` core, `h7s.2` consumers (after `h7s.1`), `h7s.3`
  ui inspector (after `h7s.2`), `h7s.4` GCP pack keys, CAI Kubernetes schemas
  and bridge rule (after `h7s.1`).
- Epic `pudl-3q5`: Kubernetes content pack (after `h7s.1`).
