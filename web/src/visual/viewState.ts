// User-controlled view state. Plain data; owned by the UI, consumed by the
// visual builder. Collapse state is keyed by semantic group ID, so it
// survives relayouts and refreshes.

import type { ResourceId } from '../model/canonical';
import type { RelationshipCategory } from '../model/vocabulary';
import type { Direction } from '../layout/types';
import { groupAncestors } from '../projection/project';
import type { SemanticView } from '../projection/types';

export interface FocusState {
  resourceId: ResourceId;
  depth: number;
}

export interface ViewState {
  collapsed: ReadonlySet<string>;
  enabledCategories: ReadonlySet<RelationshipCategory>;
  hiddenTypes: ReadonlySet<string>;
  /** Owner IDs (AWS account / GCP project) to show; null shows all. */
  scope: ReadonlySet<string> | null;
  focus: FocusState | null;
  /** Merge parallel edges of one category between the same endpoints. */
  bundleParallel: boolean;
  direction: Direction;
  /**
   * When false (default), filters are render-only: layout ignores them, so
   * filtering never moves anything. When true, filters reflow the layout.
   */
  reflowOnFilter: boolean;
}

export function initialViewState(view: SemanticView): ViewState {
  const def = view.projection;
  const collapsed = new Set<string>();
  if (def.defaultCollapsed) {
    for (const g of view.groups.values()) if (def.defaultCollapsed(g, view.vocab)) collapsed.add(g.id);
  }
  return {
    collapsed,
    enabledCategories: new Set(def.categories.enabled),
    hiddenTypes: new Set(),
    scope: null,
    focus: null,
    bundleParallel: false,
    direction: def.layout.direction,
    reflowOnFilter: false,
  };
}

/**
 * The state the layout is computed from. Unless reflowOnFilter is set,
 * filters are neutralised: every available category, no hidden types, full
 * scope. The drawn graph (built from the real state) is then a subset of the
 * laid-out one and reuses its positions.
 */
export function layoutStateOf(state: ViewState, view: SemanticView): ViewState {
  if (state.reflowOnFilter) return state;
  return {
    ...state,
    enabledCategories: new Set(view.projection.categories.available),
    hiddenTypes: new Set(),
    scope: null,
  };
}

/** Identity of a layout state: equal keys produce identical layouts. */
export function layoutKey(state: ViewState): string {
  const sorted = (s: Iterable<string>) => [...s].sort().join(',');
  return JSON.stringify([
    sorted(state.collapsed),
    sorted(state.enabledCategories),
    sorted(state.hiddenTypes),
    state.scope ? sorted(state.scope) : null,
    state.focus,
    state.bundleParallel,
    state.direction,
  ]);
}

/** The collapsed set with every ancestor group of `id` expanded, so `id` is drawn. */
export function revealed(view: SemanticView, collapsed: ReadonlySet<string>, id: ResourceId): Set<string> {
  const groupId = view.groupOf.get(id);
  const start = view.members.get(id)?.parentId ?? (groupId ? view.groups.get(groupId)?.parentId : null) ?? null;
  const next = new Set(collapsed);
  for (const gid of groupAncestors(view.groups, start)) next.delete(gid);
  return next;
}
