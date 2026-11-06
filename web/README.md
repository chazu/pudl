# pudl web: graph explorer

A browser UI for exploring a resource graph through four views (network
topology, application architecture, resource inventory, security
relationships) of the same resources. Architecture, data flow and design
rules are in [docs/frontend.md](../docs/frontend.md).

Most people never build this directly: `pudl ui` serves the bundle embedded
in the pudl binary against the workspace catalog (bannered LIVE). This
directory is the source. After changing it, run `make web` from the repository
root and commit `internal/webui/static`. Standalone, the app loads a graph
file (bannered FILE) or a synthetic demo dataset (bannered DEMO DATA).

The UI knows no resource or relationship types. What each type is called,
how it is drawn and where each view places it arrives with the graph as its
`vocabulary`, which pudl builds from CUE: schemas' `_pudl.graph` blocks and
`graph_*` datalog facts (docs/design/2026-11-02-graph-vocabulary-in-cue.md).
A new type never needs a change here. Only the styling of the fixed category
and class sets (colours, line patterns) lives in this directory.

## Run

Requires Node 20 or later.

```bash
cd web
npm install
npm run dev          # http://localhost:5178
```

Against a running `pudl ui`, with hot reload:

```bash
PUDL_API=http://127.0.0.1:7468 npm run dev
```

Or from an exported file in `web/public/` (git-ignored):

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
npm run build        # typecheck + bundle into ../internal/webui/static (embedded by pudl ui)
```

## Use

- **Views** (left): switch projection. The selection is kept, and the canvas
  centres on it if it is in the new view.
- **Search** (`/` to focus; ↑/↓, Enter, Esc): finds any resource, best
  match first. Picking a result selects it, expands its collapsed ancestors
  and centres on it at a readable zoom.
- **Scope / Resource types**: filters. By default, filtering hides things in
  place and nothing moves. Tick "Reflow on filter" to compact.
- **Canvas**: drag to pan, wheel to zoom, `f` to fit, `Esc` to clear. Click a
  node, port, group header or edge to inspect it. Double-click a group header
  (or use its −/+ box) to collapse or expand it.
- **Toolbar**: relayout from scratch, direction, expand or collapse all,
  bundle parallel edges, show unresolved references (off by default),
  focus (1 or 2 hops from the selection). The category
  chips are the legend and also toggle each category.
- **Inspector** (right): identity, placement in this view and why, which views
  the resource appears in, every relationship with its basis and how it is
  shown here, properties and evidence. For an aggregated edge, it lists each
  underlying relationship.

## Source layout

```
src/model/       canonical graph types, vocabulary (indexing and fallbacks only)
src/data/        GraphSource, validation, JSON loader, mock/ demo dataset
                 (mock/vocabulary.json stands in for pudl's vocabulary)
src/projection/  projection engine and the four view definitions
src/visual/      visual graph: visibility, collapse, ports, edge aggregation
src/layout/      LayoutEngine interface, ELK adapter, layout memory
src/render/      SVG canvas, shapes, viewport math, highlighting
src/ui/          React app, sidebar, toolbar, inspector, pipeline hook
```
