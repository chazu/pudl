# pudl ui: member_only graph flag and Pulumi inspector details (2026-11-12)

Issue: `pudl-7g8j.6` (closed). Design: `docs/design/2026-11-06-pulumi-pack.md`
§8, as-built note added.

## What was built

- **Core: `_pudl.graph.member_only`** (`internal/graphspec`): `Spec.MemberOnly`.
  A record of such a schema draws no node of its own. The graph export
  (`internal/graphexport/details.go`, `dropMemberOnly`) removes it after
  grouping when it has no representations, and counts it per schema under
  `meta.coverage.hidden` with reason `member-only`. It stays in `pudl
  list`, `show` and `query`.
- **Core: `graph_detail` relation** (`graphexport.DetailRelation`):
  `graph_detail(resource_id, section, key, value, rank?)`. The export
  evaluates it like `graph_edge_property`, resolves `resource_id` through
  the reference index (a grouped member's ID resolves to its group),
  deduplicates, and sorts by section, key, rank, value into
  `Resource.Details []Detail{Section, Key, Value, Rank}` (`details` in
  JSON). Rows naming no resource in the graph are reported as a warning.
- **pudl ui**: `Resource.details` in the canonical model;
  `web/src/model/details.ts` groups rows by section and key; the inspector
  renders one heading per section and a key table, joining ranked values
  with `›` as a path. Bundle rebuilt (`make web`).
- **Pulumi pack**:
  - `pulumi.#Resource` and `pulumi.#Stack` set `graph: member_only: true`.
  - `pulumi.#Resource` gains `name` (the URN's last segment) and `depth`
    (the URN's type chain length; 0 for the stack's root resource), set by
    the transform and projected into `pulumi_resource`.
  - Rules in `schema/pudl/rules/pulumi_pack.cue` derive the "Pulumi"
    section: `stack`, `type`, `repo`, `root`, `commit` (joined through
    `pulumi_stack_source`), `commit note` when the update was dirty, and
    `component` per ancestor with `rank` = the ancestor's depth.
- **Docs**: schema authoring (`member_only`), datalog (`graph_detail`),
  the pack README, the design as-built note.

## Tests

- `internal/graphspec`: `member_only` parses; a non-bool is rejected.
- `internal/graphexport/details_test.go`: `dropMemberOnly` keeps grouped
  records and counts the rest per schema; `applyDetails` resolves a
  member's ID to its group, drops null values, deduplicates, orders, and
  warns about orphans.
- `cmd/pulumi_pack_links_test.go`: with both packs' fixtures, the graph
  export has no `pulumi.#Resource` or `pulumi.#Stack` node; coverage hides
  13 state resources and 2 stacks as `member-only`; the linked network node
  carries the eight expected detail rows (commit note included, components
  in root-first order); the dev Deployment's rows have no commit note.
- `web`: `details.test.ts`; typecheck and the suite pass.
- `CGO_ENABLED=0 go test ./...` passes; `ratchets check` is clean.
