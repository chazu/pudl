# Graph vocabulary in CUE

Status: accepted 2026-11-02. Epic: see `br` (graph vocabulary in CUE).

## Problem

`pudl ui` draws the catalog as a graph, but what the graph *means* is written
in TypeScript:

- `web/src/model/relationshipTypes.ts`: every edge type, its category, label
  and meaning (about 40).
- `web/src/model/resourceTypes.ts`: every resource type's code, label and
  visual class (about 55).
- `web/src/projection/views/*.ts`: about 40 GCP and AWS type names that decide
  placement: which types are groups, which sit in a subnet, which are
  administrative boundaries, which are security controls, which are ports.

A content pack can add schemas, facts and `graph_edge` rules, but a new type
or a new kind of relationship still needs a TypeScript edit and a frontend
rebuild. Unregistered types render as "unknown".

## Decision

All knowledge about types and relationships lives in CUE. The UI is generic:
it receives the vocabulary with the graph and contains no resource or edge
type names.

What stays in TypeScript: view algorithms (how a placement becomes a nested
layout), and the styling of the fixed category and class sets (colour, line
pattern, port side). Their names and meanings are CUE.

### 1. Ground facts in rule files

A rule with a head and no body (`body` absent or `[]`) is a fact. Every head
argument must be a constant. It compiles to a `SELECT` with no `FROM`, can sit
beside derived rules of the same relation, and is queryable with `pudl query`.

```cue
graph_edge_type_in_subnet: {
	head: {rel: "graph_edge_type", args: {type: "in_subnet", category: "containment",
		label: "in subnet", meaning: "Has an address in this subnet."}}
}
```

### 2. Vocabulary relations

Declared as ground facts in `pudl/rules/*.cue`:

| Relation | Arguments | Declared by |
|---|---|---|
| `graph_edge_category` | `category, label, meaning, order` | pudl built-ins only |
| `graph_edge_type` | `type, category, label, meaning` | built-ins (cross-provider), packs (their own) |
| `graph_provider` | `provider, label` | built-ins (`external`), packs (`aws`, `gcp`) |
| `graph_account_type` | `provider, type` | packs: the type a provider's `account` values name (`gcp.project`, `aws.account`) |
| `graph_resource_type` | `type, code, label, class` | types with no schema: built-ins (`external.cidr`, `external.principal`), packs (`gcp.project`, `aws.account`, which they do not collect) |

Categories are the fixed set the UI styles: `containment`, `network`,
`traffic`, `dependency`, `security`, `iam`, `other`. An edge type naming
another category is a lint problem and renders as `other`.

### 3. Resource types in the schema's `_pudl.graph`

A schema's `_pudl.graph` block (already holding `name`, `account`, `region`,
`zone`, `refs`) gains display info and view roles. All new keys are optional
and strictly decoded.

```cue
graph: {
	name:    "name"
	account: "project"
	region:  {path: "region", last_segment: true}

	display: {code: "SUBN", label: "Subnetwork", class: "network"}

	// Administrative node: account, project, folder, organization.
	// parent: edges to its administrative parent; absent = top of hierarchy.
	// (admin.owner is not written here: pudl sets it on the type a
	// graph_account_type fact names.)
	admin?: {parent?: [...string]}

	// Network view. Present (or admin present) = shown in the network view.
	// group: contains other resources. via: placed under the target of the
	// first of these edges it has, else under its owner. region: wrap the
	// parent in a region group when the resource has a region.
	network?: {group?: bool, via?: [...string], region?: bool}

	// Drawn as a port on the resource this edge points to (network, security).
	port_of?: string

	// Security view: always shown, even with no security edges.
	security?: {control?: bool}

	// Inventory view: its type bucket starts collapsed.
	inventory?: {collapsed?: bool}

	// Application view: group contains members; via: placed under the target
	// of the first of these edges.
	application?: {group?: bool, via?: [...string]}
}
```

`class` is one of `admin`, `network`, `compute`, `data`, `messaging`, `edge`,
`security`, `identity`, `logical`, `external`.

The block is inherited along `base_schema` as today (the nearest block wins).
When two schemas declare the same `resource_type` with different display
info, the lexically first schema wins and lint reports the conflict. A
`graph_resource_type` fact for a type that also has a schema loses to the
schema.

### 4. Delivery to the UI

`pudl graph export` and `/api/graph` (full and partial) carry the vocabulary
as a top-level `vocabulary` field:

```json
{
  "meta": {...}, "resources": [...], "relationships": [...],
  "vocabulary": {
    "categories": [{"id": "containment", "label": "Containment", "meaning": "...", "order": 6}],
    "edgeTypes": [{"type": "in_subnet", "category": "containment", "label": "in subnet", "meaning": "...", "source": "pudl/rules/graph_vocabulary.cue"}],
    "providers": [{"provider": "gcp", "label": "Google Cloud"}],
    "resourceTypes": [{
      "type": "gcp.compute.subnetwork", "code": "SUBN", "label": "Subnetwork", "class": "network",
      "schema": "gcp.#Subnetwork",
      "admin": null,
      "network": {"group": true, "via": ["in_network"], "region": true},
      "portOf": "",
      "security": {"control": false},
      "inventory": {"collapsed": false},
      "application": null
    }],
    "problems": ["..."]
  }
}
```

Absent role blocks are `null`; absent fields inside a present block take
their zero value (lists may be `null`). Every item may carry `source` (the
fact's rule name and position); resource types from schemas carry `schema`.
A type with roles but no display has empty `code`, `label` and `class`.
`/api/graph?focus=...` without a `closure` parameter follows the
vocabulary's containment edge types. Unknown types still render (code from the type's last
segment, class `external`), and unknown edge types as category `other`.

### 5. Generic views

- **Network:** includes resources whose type has `admin` or `network`, plus
  external resources. Admin types are groups placed under `admin.parent`
  edges (root when absent); `network.group` types are groups. Placement:
  `via` edges, else owner; `region` wraps each parent in a region group.
- **Security:** includes `admin.owner` types, `security.control` types and
  anything touching a `security` or `iam` edge; `admin.owner` types are
  groups at the root.
- **Inventory:** admin types are groups (no `parent`: under a provider group;
  with `parent`: under it). Other resources: owner, region, type bucket when
  their provider has an `admin.owner` type, else the provider group.
- **Application:** `application.group` types are groups placed via
  `application.via`. Other resources go under an application group they have
  an edge to, else "not assigned".
- **Owner lookup:** a resource's `account` names a resource of the type its
  provider's `graph_account_type` fact names (`admin.owner` in the JSON).
- **Ports:** every type with `port_of`.

### 6. Tooling

- `pudl graph vocab [--json]`: the merged vocabulary, where each item was
  declared, and its problems. `--json` prints exactly the export's
  `vocabulary` field.
- `pudl doctor` graph check (a warning): a cataloged resource type with no
  display info; a `graph_edge` rule emitting a constant type with no
  `graph_edge_type`; an edge type with an unknown category; a `via`, `parent`
  or `port_of` naming an undeclared edge type; conflicting declarations. A
  provider with no `graph_provider` label is not a problem: the UI shows its
  id.
- `pudl schema new` scaffolds a `graph` block with `display` filled from the
  resource type and TODO comments for roles.
- Docs: `docs/schema-authoring.md` ("Make a type appear in pudl ui"),
  `docs/datalog.md` (ground facts), `docs/frontend.md`, and the pudl-core skill.

## Not done

Schema display info is not exposed as a datalog relation; rules cannot yet
join on a type's class. Nothing needs it today.
