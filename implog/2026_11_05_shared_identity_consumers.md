# Shared identity consumers (2026-11-05)

Issue: `pudl-h7s.2` (epic `pudl-h7s`). Design:
`docs/design/2026-11-05-shared-identity-across-schemas.md` §5.

Links from `same_resource` now show up everywhere a user looks: graph
export and pudl ui draw a group as one node, `pudl show` names the other
records, and `pudl doctor` reports what could not be linked.

## Public API

### `internal/resourcegroup` (new)

- `Load(ctx, Input{DB, Rules, Scope, SchemaOf, Metadata}) (*Index, error)`:
  evaluates `same_resource`, keeps links whose both ends are in `SchemaOf`,
  and groups them with `sameresource.Build`, namespacing by schema family.
- `FromLinks(links, namespaceOf) *Index`; `(*Index).GroupOf(resourceID)`;
  fields `Groups`, `Conflicts`.
- `Namespace(schema, meta) string`: the root of the `base_schema` chain.

### Graph export (`pudl graph export`, `/api/graph`)

- `Resource.Representations []Representation` (`representations`, omitted
  for a resource with one record): `{id, schema, type, entryId, origin}`.
- A group is one resource. Its ID is the group ID (smallest member
  `resource_id`), labels come from the member whose schema sorts first, an
  empty account, region or zone is filled from the others, and evidence and
  reference values are merged.
- References to any member's ID or refs resolve to the group, so a reference
  matching several members of one group is no longer left unresolved.
- `Subgraph` focuses a group when given a member's ID.
- A conflict, or a failure to evaluate `same_resource`, is a `meta.warnings`
  entry; resources then stay ungrouped.

### `pudl show`

- Text: "Same resource as (kinds):" then each other record's proquint and
  schema, after the compliance section.
- `--json`: `same_resource: {group_id, kinds, others: [{resource_id,
  entry_id, proquint, schema}]}`, omitted when not grouped.

### `pudl doctor`

- New check "Shared Identity" (`doctor.CheckSameResource`,
  `doctor.SameResourceInput`): warns about kinds named by a schema's
  `_pudl.same_resource` or a `same_resource` rule head that no
  `same_resource_kind` fact declares, and about conflicting components. It
  never creates a catalog.

### Docs

- `docs/schema-authoring.md` `### same_resource`; `docs/datalog.md`
  built-in relations table and "Shared identity (`same_resource`)";
  `docs/cli/notes/show.md` and `pudl show --help`; pudl-core skill.

## Tests

- `cmd/same_resource_test.go` (end to end through the CLI): a GKE cluster
  and a kube context with the same API server become one graph node with two
  representations; a workload's reference to the context points at the
  group; a member ID focuses the group in a subgraph; `pudl show` (text and
  JSON) lists the other record; doctor is ok with 1 group. A second GKE
  record with the same endpoint is a conflict: nothing is grouped, graph
  export warns, doctor reports it.
- Unit: `internal/graphexport/groups_test.go` (merge, ref resolution),
  `internal/resourcegroup` (namespace walk, `GroupOf`),
  `internal/doctor/same_resource_check_test.go`.
- `make lint`: 0 issues. Help golden file updated for `pudl show --help`.
