# pudl web: graph explorer

A browser UI for exploring a resource graph through four views (network
topology, application architecture, resource inventory, security
relationships) of the same resources. Architecture, data flow and design
rules are in [docs/frontend.md](../docs/frontend.md).

The app loads a synthetic demo dataset (bannered DEMO DATA) or a graph file
exported from a pudl workspace (bannered FILE).

## Run

Requires Node 20 or later.

```bash
cd web
npm install
npm run dev          # http://localhost:5178
```

View your own pudl catalog: export it into `web/public/` (git-ignored) and
point the app at it:

```bash
pudl graph export -o web/public/graph.json     # from your workspace
open 'http://localhost:5178/?graph=/graph.json'
```

Any URL the browser can fetch works for `?graph=`. To try it without real
infrastructure, install the AWS or GCP pack and run its fixture collectors
(see the pack READMEs), then export.

## Check

```bash
npm test             # vitest: projection, visual-graph, layout and validation tests
npm run typecheck    # tsc
npm run build        # typecheck + production bundle in web/dist
```

## Use

- **Views** (left): switch projection. The selection is kept, and the canvas
  centres on it if it is in the new view.
- **Search**: finds any resource. Clicking a result selects it, expands its
  collapsed ancestors and centres on it.
- **Scope / Resource types**: filters. By default, filtering hides things in
  place and nothing moves. Tick "Reflow on filter" to compact.
- **Canvas**: drag to pan, wheel to zoom, `f` to fit, `Esc` to clear. Click a
  node, port, group header or edge to inspect it. Double-click a group header
  (or use its −/+ box) to collapse or expand it.
- **Toolbar**: relayout from scratch, direction, expand or collapse all,
  bundle parallel edges, focus (1 or 2 hops from the selection). The category
  chips are the legend and also toggle each category.
- **Inspector** (right): identity, placement in this view and why, which views
  the resource appears in, every relationship with its basis and how it is
  shown here, properties and evidence. For an aggregated edge, it lists each
  underlying relationship.

## Source layout

```
src/model/       canonical graph types, resource and relationship registries
src/data/        GraphSource, validation, JSON loader, mock/ demo dataset
src/projection/  projection engine and the four view definitions
src/visual/      visual graph: visibility, collapse, ports, edge aggregation
src/layout/      LayoutEngine interface, ELK adapter, layout memory
src/render/      SVG canvas, shapes, viewport math, highlighting
src/ui/          React app, sidebar, toolbar, inspector, pipeline hook
```
