# Frontend: graph explorer

`web/` is a browser UI for exploring a resource graph through several views
(projections) of the same resources. It is a TypeScript + React app built
with Vite, laid out by [ELK](https://eclipse.dev/elk/) (elkjs), and rendered
as SVG. Run instructions are in [web/README.md](../web/README.md).

`pudl ui` serves it from the pudl binary against the workspace catalog
(labelled LIVE). It can also load a canonical-graph JSON file, such as the
output of `pudl graph export` (labelled FILE), or a synthetic demo dataset
(labelled DEMO DATA). See [Backend integration](#backend-integration).

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
| Sources | `src/data` | loading, validating and merging graphs (live, file, demo; partial) |
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

The graph carries its own **vocabulary** (`src/model/vocabulary.ts`): every
resource type's code, label, class and view roles, and every relationship
type's **category** and **meaning** (the strongest claim it supports, shown in
the inspector). pudl builds it from CUE (see
[Vocabulary](#vocabulary)); the UI holds no type names of its own.
Categories are a fixed set the UI styles:

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
| Security relationships | account/project; external principals and ranges apart | iam, security | layered, left→right |

The views name no resource or relationship type. Every type-specific
decision comes from the vocabulary's roles, declared in each type's schema:

- **Network:** types with `admin` or `network` roles are shown. A type's
  `network.via` edges place it, falling back to its owner, and
  `network.region` wraps the parent in a region group. That is how AWS VPCs
  sit in a region under the account, while GCP subnets sit in a region
  inside their global network.
- **Security:** account types, `security.control` types, and anything
  touching a security or iam edge.
- **Inventory:** administrative types are groups. Everything else goes
  under owner, region and type.
- **Application:** `application.group` types are groups. Members go under
  any application group they have an edge to.

A resource's owner is the type its provider's `graph_account_type` names.
Ports come from `port_of`.

### Vocabulary

`vocabulary` (built by `internal/graphvocab`, also printed by
`pudl graph vocab --json`) has `categories`, `edgeTypes`, `providers`,
`resourceTypes` and `problems`. Each resource type carries `code`, `label`,
`class`, the schema it came from, and its roles (`admin`, `network`,
`portOf`, `security`, `inventory`, `application`; `null` when absent). The
status bar shows a count of vocabulary problems and undeclared relationship
types. The demo data carries its own vocabulary
(`src/data/mock/vocabulary.json`).

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

`LayoutEngine` (`src/layout/types.ts`) is the boundary; `createLayoutEngine`
(`src/layout/engine.ts`) picks the implementation. Two ELK engines exist, and
only they know ELK's JSON format. Another engine, for example fCoSE for an
exploratory view, would implement the same interface and return the same
`LayoutResult` (absolute boxes, port boxes and edge polylines).

- **Incremental engine** (`elkIncremental.ts`), the default for layered
  views. Each group is laid out on its own (`hierarchyHandling:
  SEPARATE_CHILDREN`). An edge that crosses group boundaries is split into
  one segment per level, through generated boundary ports on each group it
  leaves or enters (exit ports on the flow's out side, entry ports on its in
  side), and the segments' routes are joined. Every level is then a flat
  graph, which lets ELK's interactive strategies run there (see below).
- **Single-pass engine** (`elkAdapter.ts`): `hierarchyHandling:
  INCLUDE_CHILDREN`, which routes everything in one pass. It is used for the
  packed inventory view (`rectpacking` per group, edges drawn straight), and
  for every view with `?engine=classic`.

Both engines use orthogonal routing, `FIXED_SIDE` ports, group padding that
reserves the header, `elk.json.shapeCoords`/`edgeCoords: ROOT` (absolute
output), Brandes–Köpf node placement, and an ELK label on aggregated edges so
room is reserved for the count pill. `NETWORK_SIMPLEX` throws on some
cross-hierarchy edges under `INCLUDE_CHILDREN`, and in the incremental engine
it was less stable than BK.

### Layout stability

These rules were measured with a sibling-order metric: the share of sibling
pairs whose left/right and above/below relation is unchanged.

1. **Model order is canonical.** Children are passed in projection order with
   `considerModelOrder: NODES_AND_EDGES`. ELK is deterministic, so an
   identical input gives an identical layout. (Sorting children by their
   previous positions made it worse and was removed.)
2. **Previous positions keep the order.** `LayoutMemory` keeps each view's
   last result and passes it as `LayoutHints.previous`. In every level where
   most children were laid out before, the incremental engine sets
   interactive layering, crossing minimisation and cycle breaking, and feeds
   each child's previous centre relative to its parent. New children go after
   everything else along the flow. Node placement stays normal: interactive
   placement treats inputs as minimums, and the diagram drifted and grew on
   every pass. ELK rejects these strategies under `INCLUDE_CHILDREN`, which
   is why the incremental engine lays out levels separately.
3. **States seen before are reproduced exactly.** `layoutSignature` hashes
   everything a layout depends on (nodes, parents, sizes, ports, edges,
   labels, config). `LayoutMemory` caches results by signature, so collapsing
   a group and expanding it again restores every position. Relayout clears
   the view's memory and starts fresh.
4. **Filters are render-only by default.** Layout runs on a graph built with
   filters neutralised (all available categories, no hidden types, full
   scope). The drawn graph is a tested subset of it, so toggling a type,
   scope or category moves nothing. "Reflow on filter" opts into compacting.
5. **Selection never relays out.**
6. **Unconnected leaves are packed in a grid** (`isolatedPacking.ts`):
   layered layout puts every edge-less node in one layer, so three or more
   per parent are replaced by a placeholder box and placed in a grid inside
   it afterwards.
7. **The canvas anchors the toggled group**, so it stays put on screen.

Measured on the demo data and the pack fixtures (incremental vs single-pass):
collapsing the largest group kept network 0.85 vs 0.73 and security 0.89 vs
0.39; a refresh that drops one resource kept network 0.87 vs 0.54 and security
0.85 vs 0.62. In the application view, with three top-level applications, one
vertical flip can cost a third of the pairs. A test requires the incremental
engine to beat the single-pass engine across these cases, with no case
below 0.6. In the browser, collapsing `prod-core` moved none of the other
top-level groups, and expanding it restored all 60 positions.

Costs: laying out levels separately is less compact. The demo views are
1.1–1.3× wider. The fixture network view is 2.2× wider and 0.8× as tall
(2780×1761 vs 1236×2177). An edge crossing several groups is drawn as
joined segments.

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
  ],
  "vocabulary": { "categories": [], "edgeTypes": [], "providers": [], "resourceTypes": [], "problems": [] }
}
```

### From a pudl workspace: `pudl graph export`

```bash
pudl graph export -o web/public/graph.json
# then open http://localhost:5178/?graph=/graph.json
```

The command (`internal/graphexport`) writes this shape from the catalog:

- **Resources** are the catalog's current entries, one per resource, except
  pudl's own bookkeeping types (`system_model`, observation records). The
  `id` is the pudl `resource_id`; `type` is the schema's `resource_type`;
  `provider` is its first segment. Name, account, region and zone come from
  the schema's [`_pudl.graph`](schema-authoring.md#graph) block. Properties are
  the record's top-level scalars (already redacted). `schema` and `entryId`
  point back at the record (`pudl show <entryId>`).
- **Evidence** for a resource is the observation snapshots that saw it,
  basis `observed`. A resource that was only ever imported gets basis
  `unknown`, because pudl did not observe it.
- **Relationships** come from rules for the
  [`graph_edge`](datalog.md#resource-graph-edges-graph_edge) relation. The
  AWS and GCP packs ship these in `schema/pudl/rules/*_graph.cue`. Each
  relationship's evidence names the rule that derived it. An unresolved
  reference becomes a `ref:<value>` endpoint, which the UI shows as not in
  the inventory.
- **Coverage** (`meta.coverage`) compares the graph with the catalog: entry
  and resource counts, the schemas whose entries are not shown (`no-identity`
  for records whose type has no identity fields, often a generic catch-all,
  and `internal` for pudl's own records), and how many `graph_edge` rules are
  loaded. The sidebar shows it, so a sparse graph explains itself. With no
  edge rules it tells you to re-run `pudl example install`, which upgrades a
  pack installed by an earlier pudl.
- **Warnings** (unreadable payloads, ambiguous references, failing rules)
  are listed in `meta.warnings` and shown in the status bar. They never stop
  the export.

The export carries the vocabulary in a top-level `vocabulary` field. A type
the vocabulary does not know still shows: a resource as unknown (code from
the type's last segment), a relationship under the `other` category
("meaning unknown"). A graph file with no vocabulary loads, and the status
bar says so.

A file is a snapshot, so the UI labels it FILE, not LIVE.

### Live: `pudl ui`

`pudl ui` serves the explorer from the binary and reads the catalog on every
load:

- **Embedded bundle.** `make web` runs Vite and writes the bundle to
  `internal/webui/static`, which is committed and embedded with `go:embed`.
  Building, installing or releasing pudl needs no Node. `make check-web`
  rebuilds and fails if the committed bundle is stale; the build is
  reproducible. The directory is not named `dist` because a common global
  gitignore pattern would drop it from commits.
- **Server** (`internal/uiserver`): `GET /api/health`, `GET /api/version`
  (the change stamp), `GET /api/graph` (the same graph as `pudl graph
  export`, `meta.origin: "live"`, `meta.version`, `no-store`), and the bundle
  with an index.html fallback. It is read-only and has no write endpoints.
- **Time travel.** `GET /api/graph?as_of=<RFC3339>` (and `pudl graph export
  --as-of`) builds the graph as pudl had recorded it at that time. Each
  resource gets its latest entry by then (`CatalogDB.ResourceEntriesAsOf`).
  Evidence lists only snapshots taken by then. Edge rules see facts as
  believed then about then (transaction and valid time both T). Rules and
  schemas are today's, and deleted entries cannot be shown. `GET
  /api/snapshots` lists recent snapshots. The UI's Time picker offers "Now",
  those snapshots and any custom time. A past view has its own banner ("AS
  RECORDED AT …") and pauses auto-refresh. View state is kept, so the same
  layout can be compared across times. Facts are timestamped in whole
  seconds, so snapshots taken in the same second are indistinguishable.
- **Large inventories: neighbourhoods on demand.** When `/api/summary`
  reports more than 500 resources (`?partialAbove=N` changes the threshold;
  `?full` disables partial loading), the UI starts empty and loads
  neighbourhoods as you go. Search runs on the server (`/api/search`).
  Picking a hit loads it and its neighbours. A `⋯N` badge on a node or group
  header, or "Load neighbours" in the inspector, loads more.
  `/api/graph?focus=&depth=&closure=` (`graphexport.Subgraph`) returns the
  focus, its N-hop neighbourhood, and the containment closure. The closure
  types come from the UI's vocabulary (`CONTAINMENT_TYPES`), so loaded
  resources still nest correctly. Only relationships whose two ends are
  loaded are returned, plus real `ref:` references. `meta.degree` gives each
  resource's full relationship count, so the UI shows "not loaded" counts and
  never mistakes an unloaded resource for one that is not in the inventory.
  Loaded neighbourhoods merge (`mergeGraphs`). Reload and auto-refresh
  re-fetch every neighbourhood loaded so far; time travel works the same way.
- **Auto-refresh.** The change stamp combines the catalog's fact write
  sequence and its entry and snapshot counters (`CatalogDB.ChangeStamp`) with
  fingerprints of the rule and schema `.cue` files. The server builds the
  graph once per stamp and serves it from cache otherwise. `GET /api/events`
  is a Server-Sent Events stream that sends the stamp on connect and on every
  change. The catalog is SQLite written by other pudl processes and cannot
  notify anyone, so the server checks the stamp every second and pushes only
  real changes. A view updated about 0.3 s after a `pudl check` finished. If
  the stream is unavailable, the UI falls back to polling `/api/version` every
  3 s. A change while the tab is hidden reloads when it is shown. Schema
  changes reset the server's cached CUE schemas, so no restart is needed. The
  "auto" box in the banner turns all of this off. Open streams end when the
  server stops, so Ctrl-C never hangs.
- **Network exposure.** It listens on `127.0.0.1:7468` by default. A
  non-loopback `--addr` needs `--allow-remote`. Requests with any other
  `Host` header get 403, so a web page cannot read the inventory through DNS
  rebinding.
- **Source selection** in the UI: `?graph=<url>` loads a file; `?demo` loads
  the demo dataset. Otherwise the UI asks `/api/health` whether pudl served
  it and, if so, loads `/api/graph`. Reload re-reads the source and keeps
  collapse state, filters and selection, because IDs are stable. The origin
  label comes from the source, never from the document.
- **Frontend development** against real data:
  `PUDL_API=http://127.0.0.1:7468 npm run dev` proxies `/api` to a running
  `pudl ui`.

## Extension points

- **New view**: add a `ProjectionDefinition` in `src/projection/views/` and
  register it in `registry.ts`.
- **New resource or relationship type**: declare it in CUE, not here: the
  schema's `_pudl: graph` block (display and roles) and `graph_edge_type`
  facts. See
  [schema authoring](schema-authoring.md#make-a-type-appear-in-pudl-ui).
- **New layout engine**: implement `LayoutEngine`, then choose it per view
  through `LayoutConfig.algorithm`.
- **Live data**: implement `GraphSource`.

## Known limitations

- Change detection is a 1 s server-side check of a cheap stamp; pudl writes
  do not notify a running `pudl ui`.
- Pack coverage: AWS IAM policy attachments and policy documents
  (`grants_on`) are not projected; GCP IAM covers project-level bindings only
  (not folder, organization or resource-level policies), and firewall rules
  that target service accounts are not matched to instances.
- Layout keeps relative order, not exact positions: after a change, nodes
  near it shift to make room. There is no manual pinning and no animation.
- With filters render-only, filtering leaves gaps until "Reflow on filter" or
  Relayout.
- Edges into an *expanded* group (peering → VPC) end on the group border via
  a logical port. ELK routes them, but a dense group can make them long.
- The inventory view draws relationship edges straight, unrouted.
- Node labels are truncated by an estimated character width, not measured text.
- The ELK bundle is about 1.4 MB and runs on the main thread. The elkjs Web
  Worker build is the next step if layouts get slow; demo layouts take
  30–150 ms.
