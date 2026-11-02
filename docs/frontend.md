# Frontend: graph explorer

`web/` is a browser UI for exploring a resource graph through several views
(projections) of the same resources. It is a TypeScript + React app built
with Vite, laid out by [ELK](https://eclipse.dev/elk/) (elkjs), and rendered
as SVG. Run instructions are in [web/README.md](../web/README.md).

pudl has no HTTP API today, so the UI runs on a synthetic demo dataset (always
labelled DEMO DATA) or on a canonical-graph JSON file. See
[Backend integration](#backend-integration).

## Data flow

```
GraphSource ──► CanonicalGraph ──► project() ──► SemanticView
 (mock/file)    (resources,        (projection    (groups, members,
                 relationships)     definition)     candidate relationships,
                                                    dispositions)
                                                        │
                    ViewState (collapse, filters, focus)┤
                                                        ▼
                                   buildVisualGraph() ──► VisualGraph
                                                         (nodes, groups, ports,
                                                          aggregated edges)
                                                        │
                                                        ▼
                                   LayoutEngine.layout() ──► LayoutResult
                                   (ElkLayoutEngine)        (boxes, port boxes,
                                                             edge routes)
                                                        │
                                                        ▼
                                                   GraphCanvas (SVG)
```

Each stage is a pure function of the previous one plus explicit state, and
each lives in its own directory:

| Stage | Directory | Knows about |
| --- | --- | --- |
| Canonical model | `src/model` | resources, relationships, evidence; type registries |
| Sources | `src/data` | loading and validating a canonical graph |
| Projection | `src/projection` | grouping, inclusion, relationship selection |
| Visual model | `src/visual` | nodes, groups, ports, edges, collapse, filters |
| Layout | `src/layout` | ELK (only `elkAdapter.ts`) |
| Rendering | `src/render` | SVG, pan/zoom, highlighting |
| UI | `src/ui` | React state, sidebar, toolbar, inspector |

`src/ui/usePipeline.ts` wires the stages together.

## Canonical graph

`src/model/canonical.ts`:

- **Resource**: stable `id`, PUDL `type` (`aws.ec2.instance`, the same
  `resource_type` strings as the content packs), `name`, `provider`,
  `nativeId`, `account` (AWS account or GCP project), `region`, `zone`,
  `properties`, `tags`, `evidence[]`.
- **Relationship**: `id`, `type`, `source`, `target`, `basis`, `properties`,
  `evidence[]`. Directed and typed.
- **Basis**: `observed` (seen by a collector), `declared` (configuration or a
  manifest), `inferred` (a rule or heuristic), `unknown`.
- **Evidence**: basis, source (collector, file or rule), time, snapshot, detail.

Canonical objects carry no positions, visual parents, ports or routes, and no
stage mutates them (a test checks this).

`src/model/relationshipTypes.ts` registers every relationship type with one
**category** and a **meaning** (the strongest claim it supports, shown in the
inspector):

| Category | Meaning | Line |
| --- | --- | --- |
| network | configured connectivity; not reachability | solid blue |
| traffic | communication recorded by a collector | thick solid green |
| dependency | configured or declared dependency; not observed traffic | long-dash amber |
| security | a control or rule; not a connection | dashed red |
| iam | attachment, binding or trust; not effective permission | dotted violet |
| containment | placement or membership | short-dash grey |
| other | unregistered type; meaning unknown | dash-dot grey |

Unregistered types fall into `other`. They are never guessed into a stronger
category.

## Projections

A `ProjectionDefinition` (`src/projection/types.ts`) declares:

- `include(resource)`: which resources the view shows.
- `isGroup(resource)`: which resource types become compound groups.
- `place(resource)`: the resource's parents in *this* view, as resources or
  synthetic groups (region, type bucket, provider, "outside the inventory
  boundary"). Each placement records its **basis**, shown in the inspector.
- `ports`: resources drawn as ports on a host (ENIs on instances).
- `categories.available` / `categories.enabled`: which relationship
  categories the view can draw, and which are on by default.
- `flowCategories`: categories that read as flow (in on one side, out on the
  other); the rest attach to a side "control" port.
- `layout`: algorithm and direction.

`project()` (`src/projection/project.ts`) applies a definition. Rules:

- **Containment is a property of the view.** An ETL instance in a GCP
  Shared VPC sits under `acme-net-host / shared-vpc / us-central1 /
  analytics-usc1` in the network view, under `acme-analytics / us-central1 /
  Compute instance` in the inventory view, and under `Analytics / etl` in the
  application view.
- **Several parents → lowest common ancestor, with a note.** The ALB in two
  subnets is drawn in the VPC; the cross-account peering connection is drawn at
  the top level. The engine never picks one parent arbitrarily.
- **Placement cycles terminate.** A group already being resolved is not
  re-entered.
- **Every relationship gets a disposition**: candidate, port binding,
  category not in this view, or endpoint not in this view.
- **Unresolved references stay unknown.** A relationship whose other end is
  not in the inventory gets a placeholder in a "Not in inventory" group. The
  placeholder carries only the ID: no type, owner or connections.

The four views (`src/projection/views/`):

| View | Grouping | Edges on by default | Layout |
| --- | --- | --- | --- |
| Network topology | AWS: account → region → VPC → subnet. GCP: org → folder → project → network → region → subnet | network, traffic, security, containment | layered, left→right |
| Application architecture | application → service → member; "not assigned" for dependency participants without membership | dependency, traffic | layered, left→right |
| Resource inventory | provider → account/project (GCP: org → folder → project) → region → type | none | packed |
| Security relationships | account/project; external principals and ranges apart | iam, security | layered, top→bottom |

The network view is provider-aware. AWS VPCs are regional, so the region sits
between account and VPC. GCP networks are global and subnets regional, so the
region sits inside the network.

## Visual model

`buildVisualGraph()` (`src/visual/build.ts`, `edges.ts`, `visibility.ts`)
turns a semantic view and a `ViewState` into visual nodes and edges. Every
visual element points back at canonical IDs (`resourceId`,
`relationshipIds`).

- **Node kinds**: `resource`, `group` (expanded compound), `collapsed`
  (group drawn as a leaf, with member and internal-relationship counts),
  `unresolved`.
- **Ports**: there are three kinds, drawn differently.
  - *Semantic* ports are real interfaces, such as an ENI on its instance
    (circle, labelled `eth0`). Relationships of the interface start there.
  - *Logical* ports are category attachment points, one per node, category and
    direction (small square). Flow categories use the in/out sides; other
    categories use the control side.
  - *Boundary* ports are generated on a collapsed group where edges to its
    contents end (hollow diamond). The tooltip and inspector say a boundary
    port is a drawing device, not a network hop.
- **Edge lifting and aggregation.** An endpoint inside a collapsed group is
  lifted to the group. Lifted edges, and parallel edges when "Bundle parallel"
  is on, are merged by (source port, target port, category). Edges are never
  merged across categories. A merged edge lists every underlying
  relationship, and the inspector shows each one with its own basis and
  evidence.
- **Visual dispositions.** Every candidate relationship is drawn,
  internal to a collapsed group (listed on the group), shown as nesting
  (containment to an enclosing group), to-enclosing-group (not drawn, counted),
  category filtered, or endpoint hidden (with a reason). The inspector shows
  this for every relationship of the selected resource.
- **Filters only remove.** Hiding a resource removes its relationships.
  Nothing is rerouted around it, so hiding B never produces an apparent A→C
  edge. Hiding a group's type flattens the group, and its members move up to
  the nearest drawn ancestor.
- **Focus** keeps the selected resource's 1- or 2-hop neighbourhood over
  enabled relationships. Ports are folded into their hosts.

Tests assert these properties over all four views:
`src/visual/build.test.ts` checks faithfulness (every drawn relationship's
endpoints map to the edge's endpoints and share its category) and
conservation (every candidate has exactly one disposition).

## Layout

`LayoutEngine` (`src/layout/types.ts`) is the boundary. `ElkLayoutEngine`
(`src/layout/elkAdapter.ts`) is the only code that knows ELK's JSON format.
Another engine, for example fCoSE for an exploratory view, would implement the
same interface and return the same `LayoutResult` (absolute boxes, port boxes
and edge polylines).

The ELK configuration for the layered views:

- `elk.algorithm: layered` with `hierarchyHandling: INCLUDE_CHILDREN`, so
  edges between nodes in different nested groups are routed in one pass.
- Orthogonal routing; ports are `FIXED_SIDE`; group padding reserves the header.
- `elk.json.shapeCoords` / `edgeCoords: ROOT`, so all coordinates come back
  absolute.
- Node placement is `BRANDES_KOEPF`. `NETWORK_SIMPLEX` throws
  `NEdge must have a source and target NNode specified` on some cross-hierarchy
  edges in a top-to-bottom hierarchical layout (the security view).
- Aggregated edges get an ELK label, so ELK reserves room for the count pill.

The inventory view uses `rectpacking` per group and draws edges straight (they
are off by default).

### Layout stability

These rules were measured on the demo data with a sibling-order metric (the
share of sibling pairs whose left/right and above/below relation is
unchanged).

1. **ELK is deterministic, so model order is canonical.** Children are always
   passed in projection order with `considerModelOrder: NODES_AND_EDGES`, and
   the same state reproduces the same layout exactly. An earlier version sorted
   children by their previous on-screen position. That changed the model order
   and kept only 39–54% of sibling order on an identical rerun, so it was
   removed.
2. **Filters are render-only by default.** Layout runs on a graph built with
   filters neutralised (all available categories, no hidden types, full
   scope). The drawn graph is a subset of that graph (a test checks nodes,
   ports and edges in all four views), so toggling a type, scope or category
   moves nothing. Small filter changes can otherwise reorder a whole group:
   hiding one KMS key kept only 31–36% of sibling order, because ELK's
   hierarchical layer sweep settles on a different local optimum. "Reflow on
   filter" opts into compacting.
3. **Selection never relays out.** Highlighting is computed from the visual
   graph.
4. **Collapse and expand relay out, with an anchor.** The canvas keeps the
   toggled group fixed on screen. Collapsing the largest VPC keeps 77% of
   sibling order; the security and application views keep 83–99%. A test pins
   these thresholds.
5. ELK's interactive layering and crossing minimisation are not supported
   with `INCLUDE_CHILDREN`. `semiInteractive`, `forceNodeModelOrder`,
   thoroughness and every other layering strategy made no material difference.

`LayoutMemory` (`src/layout/layoutMemory.ts`) keeps the last result per view
and passes it to the engine as `LayoutHints.previous`. The ELK adapter ignores
it today. It is the seam for incremental work such as pinning or relaying out
only the changed subtree.

## Semantic correctness rules

- Categories are visually distinct by colour *and* line pattern, and are
  never merged.
- Each relationship type's `meaning` states what it does not establish
  (peering is not reachability, an SG association is not a connection, a
  policy attachment is not effective permission, a declared dependency is not
  observed traffic). The inspector shows it with every edge.
- `observed_flow` (traffic) is a separate category from declared
  dependencies, so both can appear between the same pair.
- Synthetic groups (regions, type buckets) have dashed outlines and say
  "Grouping (not a resource)".
- Placement notes explain any placement that is not a single obvious parent.
- Unresolved references are placeholders, not invented resources.
- Mock and file data are always bannered. A file can never claim to be
  live: `parseCanonicalGraph` forces `origin: 'file'`.

## Backend integration

`GraphSource` (`src/data/source.ts`) is the only integration point:

```ts
interface GraphSource {
  readonly id: string;
  readonly label: string;
  load(): Promise<CanonicalGraph>;
}
```

Shipped sources:

- `MockGraphSource`: the deterministic demo dataset in `src/data/mock/`
  (AWS: two accounts, three regions, peering, IAM with cross-account trust;
  GCP: org → folder → host and service projects with a Shared VPC; logical
  applications; a cross-provider dependency; two dangling references).
- `JsonFileSource`: `?graph=<url>` loads a JSON document shaped like
  `CanonicalGraph`:

```json
{
  "meta": { "label": "export 2026-11-02" },
  "resources": [
    { "id": "aws:i-0abc", "type": "aws.ec2.instance", "name": "api-1",
      "provider": "aws", "account": "111111111111", "region": "us-east-1",
      "properties": {}, "evidence": [{ "basis": "observed", "source": "aws ec2 describe-instances" }] }
  ],
  "relationships": [
    { "id": "r1", "type": "in_subnet", "source": "aws:i-0abc", "target": "aws:subnet-0def", "basis": "observed" }
  ]
}
```

A live source would map pudl's catalog to this shape: catalog entries become
resources (`resource_type`, identity, payload fields), facts become
relationships (a relation such as `aws_instance_sg` becomes `protected_by`),
and observation snapshots become evidence. That needs a read API, which pudl
does not have yet, for example a `pudl graph export --json` command or a local
HTTP endpoint over `pkg/factstore`. Live data would set `origin: 'live'`.

## Extension points

- **New view**: add a `ProjectionDefinition` in `src/projection/views/` and
  register it in `registry.ts`.
- **New resource or relationship type**: add it to `resourceTypes.ts` or
  `relationshipTypes.ts`. A relationship type needs a category and a meaning.
- **New layout engine**: implement `LayoutEngine`, then choose it per view
  through `LayoutConfig.algorithm`.
- **Live data**: implement `GraphSource`.

## Known limitations

- No live inventory; see above.
- Collapse and expand still move nodes outside the toggled group (anchored,
  77–99% sibling order kept). There is no pinning and no animation.
- A refreshed inventory relays out from scratch. That is deterministic, but a
  small change can reorder a large group.
- With filters render-only, filtering leaves gaps until "Reflow on filter" or
  Relayout.
- Edges into an *expanded* group (peering → VPC) end on the group border via
  a logical port. ELK routes them, but a dense group can make them long.
- The inventory view draws relationship edges straight, unrouted.
- Node labels are truncated by an estimated character width, not measured text.
- The ELK bundle is about 1.4 MB and runs on the main thread. The elkjs Web
  Worker build is the next step if layouts get slow; demo layouts take
  30–150 ms.
