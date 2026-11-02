# Frontend: interactive graph explorer (web/)

First browser UI for pudl: a TypeScript + React + Vite app in `web/` that
shows the same canonical resources through four projections, laid out by ELK
and rendered as SVG. pudl has no HTTP API, so the app runs on a clearly
bannered synthetic dataset or a canonical-graph JSON file (`?graph=<url>`).
Design and data flow: [docs/frontend.md](../docs/frontend.md).

## Public API (TypeScript, `web/src`)

- `model/canonical.ts`: `CanonicalGraph`, `Resource`, `Relationship`,
  `Evidence`, `Basis`, `GraphIndex` (read-only lookups `resource`, `out`,
  `in`, `targets`, `sources`).
- `model/relationshipTypes.ts`: `relationshipType(id)` gives category, label
  and meaning; unregistered types get category `other`. `CATEGORIES`,
  `CATEGORY_ORDER`.
- `model/resourceTypes.ts`: `resourceType(type)` gives code, label and class.
- `data/source.ts`: `GraphSource` (integration point), `loadFrom`,
  `sourceFromLocation`. `data/validate.ts`: `validateGraph`,
  `parseCanonicalGraph` (forces `origin: 'file'`). `data/mock`:
  `buildDemoGraph`, `MockGraphSource`. `data/jsonSource.ts`: `JsonFileSource`.
- `projection/types.ts`: `ProjectionDefinition`, `SemanticView`,
  `SemanticGroup`, `SemanticMember`, `ProjectionDisposition`.
  `projection/project.ts`: `project(index, def)`, `groupAncestors`,
  `lowestCommonAncestor`. `projection/registry.ts`: `PROJECTIONS`,
  `projectionById`. Views: `networkProjection`, `applicationProjection`,
  `inventoryProjection`, `securityProjection`.
- `visual/types.ts`: `VisualGraph`, `VisualNode`, `VisualEdge`, `VisualPort`
  (`semantic | logical | boundary`), `EdgeDisposition`.
  `visual/build.ts`: `buildVisualGraph(view, index, state)`.
  `visual/viewState.ts`: `ViewState`, `initialViewState`, `layoutStateOf`,
  `layoutKey`, `revealed`.
- `layout/types.ts`: `LayoutEngine`, `LayoutConfig`, `LayoutResult`,
  `LayoutHints`. `layout/elkAdapter.ts`: `ElkLayoutEngine`, `toElkGraph`,
  `fromElkGraph`. `layout/layoutMemory.ts`: `LayoutMemory`, `displacement`.

## Decisions and findings

- Containment belongs to the projection. A resource with several placement
  parents goes to their lowest common ancestor, with a note; placement cycles
  terminate.
- Every relationship gets a recorded disposition in the projection and again
  in the visual graph. Tests check faithfulness and conservation over all four
  views.
- Edges are aggregated only within a category, and only when an endpoint is
  lifted into a collapsed group or "Bundle parallel" is on.
- ELK: `NETWORK_SIMPLEX` node placement throws on cross-hierarchy edges with
  `INCLUDE_CHILDREN` in a top-to-bottom layout, so the adapter uses
  `BRANDES_KOEPF`. Interactive strategies are unsupported with
  `INCLUDE_CHILDREN`.
- Stability: sorting children by their previous positions made layouts *less*
  stable, so model order is canonical (ELK is deterministic). Small filter
  changes reshuffled whole groups (31–36% sibling order kept), so filters are
  render-only by default: layout ignores them and the drawn graph is a tested
  subset of the laid-out graph.

## Verification

- `npm test`: 41 vitest tests (projection, visual graph, ELK adapter, validation).
- `npm run typecheck`, `npm run build`: clean.
- `CGO_ENABLED=0 go test ./...`: passes (no Go changes).
- Checked in headless Chromium: all four views, selection and inspector,
  collapse with boundary ports and aggregated edges, search and reveal, view
  switching that keeps the selection, filters that move nothing, the JSON file
  source, and dark mode.
