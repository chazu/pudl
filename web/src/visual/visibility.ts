// Which members and groups are visible under the current filters and focus.
//
// Filters only ever remove things. A hidden resource takes its relationships
// with it; nothing is rerouted around it, so hiding B never creates an
// apparent A→C edge.

import type { GraphIndex, ResourceId } from '../model/canonical';
import { groupAncestors } from '../projection/project';
import type { SemanticView } from '../projection/types';
import type { ViewState } from './viewState';

export interface Visibility {
  members: Set<ResourceId>;
  /** Semantic groups that are drawn (expanded or collapsed). */
  groups: Set<string>;
  /** Why a member or group resource is hidden. */
  hiddenReason: Map<ResourceId, string>;
  /** Nearest drawn ancestor group of a semantic parent ID. */
  drawnParent(parentId: string | null): string | null;
}

export function filterReason(index: GraphIndex, id: ResourceId, state: ViewState): string | null {
  const r = index.resource(id);
  if (!r) return null;
  if (state.hiddenTypes.has(r.type)) return `type ${r.type} is filtered out`;
  if (state.scope && r.account && !state.scope.has(r.account)) return `owner ${r.account} is out of scope`;
  return null;
}

export function computeVisibility(view: SemanticView, index: GraphIndex, state: ViewState): Visibility {
  const hiddenReason = new Map<ResourceId, string>();
  const groupPasses = new Map<string, boolean>();
  for (const g of view.groups.values()) {
    const reason = g.resourceId ? filterReason(index, g.resourceId, state) : null;
    if (reason && g.resourceId) hiddenReason.set(g.resourceId, reason);
    groupPasses.set(g.id, reason === null);
  }

  const neighborhood = state.focus ? focusNeighborhood(view, state) : null;

  // A placeholder for an uncollected resource exists only to anchor its
  // relationships; without an enabled one it would float unexplained.
  const anchored = new Set<ResourceId>();
  for (const rel of view.candidates) {
    if (!state.enabledCategories.has(view.vocab.relationshipType(rel.type).category)) continue;
    anchored.add(rel.source);
    anchored.add(rel.target);
  }

  const members = new Set<ResourceId>();
  for (const m of view.members.values()) {
    if (m.role === 'unresolved' && !anchored.has(m.resourceId)) {
      hiddenReason.set(m.resourceId, 'no relationship to it is shown');
      continue;
    }
    const reason = filterReason(index, m.resourceId, state);
    if (reason) {
      hiddenReason.set(m.resourceId, reason);
      continue;
    }
    const anchor = m.role === 'port' && m.hostId ? m.hostId : m.resourceId;
    if (neighborhood && !neighborhood.has(anchor)) {
      hiddenReason.set(m.resourceId, 'outside the focused neighbourhood');
      continue;
    }
    members.add(m.resourceId);
  }
  // Ports need a visible host.
  for (const m of view.members.values()) {
    if (m.role === 'port' && m.hostId && members.has(m.resourceId) && !members.has(m.hostId)) {
      members.delete(m.resourceId);
      hiddenReason.set(m.resourceId, 'its host is hidden');
    }
  }

  // A group is drawn if it passes filters and either holds something visible
  // or is itself a resource worth showing (an empty subnet is information).
  const keep = new Set<string>();
  const markChain = (parentId: string | null) => {
    for (const gid of groupAncestors(view.groups, parentId)) keep.add(gid);
  };
  for (const id of members) markChain(view.members.get(id)?.parentId ?? null);
  for (const g of view.groups.values()) {
    if (!g.resourceId || !groupPasses.get(g.id)) continue;
    const inFocus = !neighborhood || neighborhood.has(g.resourceId);
    if (inFocus) markChain(g.id);
    else hiddenReason.set(g.resourceId, 'outside the focused neighbourhood');
  }
  const groups = new Set<string>();
  for (const gid of keep) if (groupPasses.get(gid)) groups.add(gid);

  const drawnParent = (parentId: string | null): string | null =>
    groupAncestors(view.groups, parentId).find((gid) => groups.has(gid)) ?? null;

  return { members, groups, hiddenReason, drawnParent };
}

/**
 * Resources within `depth` hops of the focus over enabled candidate
 * relationships, ports folded into their hosts. Focusing a group resource
 * seeds with everything placed inside it.
 */
function focusNeighborhood(view: SemanticView, state: ViewState): Set<ResourceId> {
  const focus = state.focus!;
  const anchor = (id: ResourceId) => {
    const m = view.members.get(id);
    return m?.role === 'port' && m.hostId ? m.hostId : id;
  };
  const adj = new Map<ResourceId, Set<ResourceId>>();
  const link = (a: ResourceId, b: ResourceId) => {
    if (!adj.has(a)) adj.set(a, new Set());
    adj.get(a)!.add(b);
  };
  for (const rel of view.candidates) {
    if (!state.enabledCategories.has(view.vocab.relationshipType(rel.type).category)) continue;
    const a = anchor(rel.source);
    const b = anchor(rel.target);
    link(a, b);
    link(b, a);
  }
  const seeds = new Set<ResourceId>([anchor(focus.resourceId)]);
  const focusGroup = view.groupOf.get(focus.resourceId);
  if (focusGroup) {
    for (const m of view.members.values()) {
      if (groupAncestors(view.groups, m.parentId).includes(focusGroup)) seeds.add(anchor(m.resourceId));
    }
    for (const g of view.groups.values()) {
      if (g.resourceId && groupAncestors(view.groups, g.id).includes(focusGroup)) seeds.add(g.resourceId);
    }
  }
  const seen = new Set(seeds);
  let frontier = [...seeds];
  for (let d = 0; d < focus.depth; d++) {
    const next: ResourceId[] = [];
    for (const id of frontier) {
      for (const n of adj.get(id) ?? []) {
        if (!seen.has(n)) {
          seen.add(n);
          next.push(n);
        }
      }
    }
    frontier = next;
  }
  return seen;
}
