# Graph explorer usability on large real inventories (pudl-azvs)

Date: 2026-11-05

## Problem

A usability review of `pudl ui`, rendered against a real multi-project GCP
catalog, found the explorer hard to use above the partial-loading threshold.
It opened on an empty canvas. Loading a project brought only the project,
because a project's resources are tied to it by their `account` attribute,
which no relationship reaches. `?full` produced a graph too large to fit, and
the "Not in inventory" group made it a tall column. Search was unranked, so
an exact name could be missing from the visible results. Picking a result
left it a speck at the fit zoom. Long cloud names, which share prefixes,
were cut at the end until they could not be told apart. Sidebar lists
crowded out the hierarchy, the toolbar and status bar clipped, and the
inspector always took 360px. Fifteen issues, pudl-azvs.1 to .15.

## Change

### Getting to a useful picture

- Partial mode opens on the administrative overview: organizations, folders,
  accounts and projects, each a box with a `⋯N` badge.
- An owner's badge, or "Load contents" in the inspector, loads what it owns.
  The expanded node keeps its place on screen.
- A view with more than `LARGE_VIEW` (400) resources opens with owner,
  external and unresolved groups collapsed. `MIN_ZOOM` is 0.02, so Fit
  always fits.
- Search ranks exact name, then name prefix, name substring, native ID, and
  type or ID, with shorter names first. The server and the local search use
  the same order.
- Picking a result centres it at a readable zoom (`READABLE_ZOOM` 0.8). The
  centring waits for a layout that places the node.

### Reading

- Labels on the canvas and in search results are cut in the middle. Search
  results show type and owner under the name. Group-header sublabels fit the
  room the label and badge leave. Expanded groups are at least 260px wide.
- An empty view says why in every mode, names the selected resource when it
  is not in the view, and offers buttons for the views that show it.
- "Not in inventory" placeholders are hidden by default. The setting is part
  of the layout, so hiding them leaves no gap.
- The inspector folds Native ID, PUDL ID and `pudl show` into a collapsed
  Identifiers block with copy buttons.
- The status bar lists problems first and wraps.

### Layout

- Search: `/` focuses it; ↑/↓, Enter and Esc work in it. Results close once
  a resource is picked.
- Sidebar sections fold from their headings, remembered per browser. Scope
  and Resource types show a filter summary such as "12 of 55". Long lists
  get a filter box, and all/none buttons. Scope starts folded when long.
- The time picker is one row. The date-time input appears only for "Custom
  time…".
- The inspector hides with » or `i`, starts hidden below 1200px wide, and
  opens on selection.
- Direction, bundle parallel, unresolved references and reflow are in a
  toolbar Options menu, which shows a dot when any is changed. Legend chips
  wrap.

## Public API

HTTP (`internal/uiserver`):

- `GET /api/graph?overview=1`: the administrative resources plus their
  containment closure (`graphexport.OverviewIDs`).
- `GET /api/graph?owner=<id,...>`: owner resources and everything they own,
  without neighbours (`graphexport.OwnedIDs`).
- `/api/search` uses the new ranking (`searchRank`).

Go (`internal/graphexport`):

- `OverviewIDs(g *Graph) []string` and `OwnedIDs(g *Graph, owners []string) []string`
  (`owners.go`).
- `Meta.Owned map[string]int` (`json:"owned"`): for each owner in a partial
  graph, how many resources it owns in the whole graph. `Subgraph` sets it.

Web (`web/src`):

- `GraphSource.overview?` and `GraphSource.owned?`, implemented by
  `LiveGraphSource`. `GraphMeta.owned`.
- `merge.ts`: `notLoadedRelationships`, `notLoadedOwned`; `notLoaded` is
  their sum.
- `ViewState.showUnresolved`; `LARGE_VIEW`, `isOwnerGroup` (`viewState.ts`).
- `render/text.ts` `middleEllipsis`; `viewport.ts` `READABLE_ZOOM` and
  `centerOn(..., minZoom)`; `CanvasHandle.centerOnNode` returns whether it
  could centre.
- New components: `SearchBox`, `searchRank`, `EmptyViewHint`,
  `SidebarSection`, `FilterLists` (`ScopeSection`, `TypeSection`),
  `OptionsMenu`, plus `Identifiers` and `CopyButton` in `inspectorParts`.

## Verification

- `npm test` passes, with new tests for search ranking, middle ellipsis,
  `centerOn`, owned counts and large-view collapse. `npm run typecheck`
  passes.
- `go test ./internal/graphexport/ ./internal/uiserver/` passes, with new
  tests for the overview, owned IDs and counts, ranking, and the new
  `/api/graph` parameters.
- Checked by driving headless Chrome against a live `pudl ui` on the global
  catalog, at 1440×900 and 1024×768, in partial and `?full` modes.
