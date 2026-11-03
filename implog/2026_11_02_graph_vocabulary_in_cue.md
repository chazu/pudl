# Graph vocabulary in CUE (pudl-mrw)

pudl ui used to know types from TypeScript: `web/src/model/relationshipTypes.ts`
(edge types, categories, meanings), `web/src/model/resourceTypes.ts`
(code, label, class) and about 40 type names in the four views. A pack could
add schemas and `graph_edge` rules but not a new kind of type or edge.
Everything is now CUE, and the UI is generic. Design:
`docs/design/2026-11-02-graph-vocabulary-in-cue.md`.

## What changed

- **Datalog facts** (`internal/datalog`): a rule with no body (absent or
  `[]`) is a fact; every head argument must be a constant (`checkFact`). It
  compiles to a `SELECT` with no `FROM`, in both the acyclic and the
  stratified evaluator. The bootstrap `#Rule` makes `body` optional.
- **`_pudl.graph`** (`internal/graphspec/roles.go`): strictly decoded
  `display {code, label, class}` (code ≤ 5 characters, class from a closed
  list) and view roles: `admin {parent}`, `network {group, via, region}`,
  `port_of`, `security {control}`, `inventory {collapsed}`,
  `application {group, via}`.
- **Vocabulary** (`internal/graphvocab`): merges schema graph blocks
  (inherited along `base_schema`) with facts of `graph_edge_category`,
  `graph_edge_type`, `graph_provider`, `graph_account_type` and
  `graph_resource_type`. It reports problems: undeclared edge types (in
  `graph_edge` rules and in roles), unknown categories, conflicting
  declarations, vocabulary rules with bodies, and cataloged types with no
  display info. `graph_account_type` sets `admin.owner` on the type a
  provider's `account` values name, so packs need no account schema.
- **Export** (`internal/graphexport`): `Graph.Vocabulary`, kept by
  `Subgraph`. `/api/graph?focus=` without `closure` follows the vocabulary's
  containment edge types.
- **CUE data**: `pudl/rules/graph_vocabulary.cue` (bootstrap) holds the 7
  categories, 36 cross-provider edge types, and the `external` provider and
  types. The GCP pack gains display and roles on 18 types, plus the
  provider, account type and `gcp.project` display. The AWS pack gains
  display and roles on 11 types, its 5 own edge types (`in_vpc`,
  `protected_by`, `has_instance_profile`, `profile_role`,
  `trust_allows_assume`), plus the provider, account type and `aws.account`
  display.
- **UI** (`web/src`): `model/vocabulary.ts` replaces both registries. It
  builds a `Vocab` index (`GraphIndex.vocab`) from the graph, with fallbacks
  for unknown types. Views read roles; none names a resource or edge type.
  Category and class *styling* stays in TypeScript. The demo data carries
  `data/mock/vocabulary.json`. The status bar counts vocabulary problems and
  undeclared edge types. The bundle is rebuilt.
- **Tooling**: `pudl graph vocab [--json]` (the JSON is exactly the export's
  `vocabulary`), the doctor check **Graph Vocabulary** (warning; it does not
  create a catalog), and a `graph` block scaffolded by `pudl schema new`
  (`internal/schemagen/graph_block.go`: name, account and region from the
  data, display guessed from the definition name, roles as commented
  examples).
- **Docs**: `docs/schema-authoring.md` "Make a type appear in pudl ui",
  `docs/datalog.md` (Facts, Resource graph vocabulary), `docs/frontend.md`,
  the pudl-core skill, the regenerated CLI reference and help golden.

## Public API

- `datalog`: rules with an empty body are facts (`checkFact`).
- `graphspec`: `Display`, `Admin`, `NetworkRole`, `SecurityRole`,
  `InventoryRole`, `ApplicationRole`, `Roles` (`EdgeTypes()`),
  `Spec.Display`, `Spec.Roles`, `Classes`, `MaxCodeLen`, `CheckDisplay`.
- `graphvocab`: `Build(Input) *Vocabulary`; `Vocabulary`, `Category`,
  `EdgeType`, `Provider`, `ResourceType`; `SpecFor`, `ProviderOf`;
  `(*Vocabulary).MissingDisplay`, `NoteMissingDisplay`, `EdgeTypesIn`; the
  relation-name constants.
- `graphexport`: `Graph.Vocabulary`, `Input.Schemas`, `Vocabulary(...)`.
- `doctor.CheckGraphVocabulary`.
- CLI: `pudl graph vocab`.

## Behaviour differences in the views

- A resource whose `via` edges are all missing is placed under its owner,
  not at the top level.
- A region group wraps a parent only when the resource has a region. There
  is no "unknown region" group any more.
- External nodes appear in a view when they touch a category the view
  offers.
- In the application view, any edge to an `application.group` resource
  counts as membership.
- Placement explanations are generic ("Placed by in_subnet.").

## Verification

- `go test ./...` passes, except `internal/proc TestCommandCancelSendsSIGTERM`,
  which is flaky under load and passes alone.
- `make lint` reports 0 issues; web: `tsc` clean, vitest 63/63.
- The pack export tests assert a complete vocabulary: no problems, display
  for every exported type, and every relationship type declared.
- `pudl ui` on a workspace with both packs' fixture data: 99 resources, 108
  relationships, no unknown or undeclared types, rendered with pack codes and
  nesting.

## Not done

- Schema display info is not a datalog relation, so rules cannot join on a
  type's class.
- A `graph_resource_type` fact carries display only, no roles. Types without
  a schema cannot declare `admin.parent`, so `gcp.project` sits at the top
  of the hierarchy.
