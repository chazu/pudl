# pudl ui: "only" next to each filter checkbox (2026-11-05)

Issue: `pudl-4lh`.

Each checkbox in the sidebar's **Scope** (accounts/projects) and **Resource
types** lists has an "only" button. It shows on hover or keyboard focus.
Clicking it isolates that item; the button then reads "all" (accent colour,
always visible), and clicking it again shows everything. The toolbar and
auto-refresh checkboxes are settings, not filters, and have none.

Type isolation hides every other type in the whole graph, not just the
current view's list, so it holds when the view changes.

## Public API (web)

- `web/src/ui/filters.ts` (new, pure): `onlyOwner(scope, owner)`,
  `isOnlyOwner(scope, owner)`, `onlyType(hidden, type, allTypes)`,
  `isOnlyType(hidden, type, allTypes)`.
- `web/src/ui/Sidebar.tsx`: `OnlyButton` beside each scope and type
  checkbox.
- `web/src/styles.css`: `.checks li` is a flex row; `.checks .only`.

## Tests

- `web/src/ui/filters.test.ts`: isolate, switch, and restore for owners and
  types. vitest 69 passed; `tsc -b` clean; `internal/webui` tests pass.
- Bundle rebuilt (`internal/webui/static`); `make install` updated
  `~/go/bin/pudl`.
- Not checked in a browser.
