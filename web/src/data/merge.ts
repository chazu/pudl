// Partial graphs: merge neighbourhoods loaded on demand, and count what is
// not loaded yet. A relationship to an unloaded resource is never in the
// graph, so it can never be mistaken for one to an uncollected resource.
// Owners (accounts, projects) also count the resources they own that are
// not loaded: ownership is an attribute, so relationships never reach them.

import type { CanonicalGraph, GraphIndex, ResourceId } from '../model/canonical';

/** Union of two graphs by ID; `add` wins on conflicts and supplies the meta and vocabulary. */
export function mergeGraphs(base: CanonicalGraph, add: CanonicalGraph): CanonicalGraph {
  const resources = new Map(base.resources.map((r) => [r.id, r]));
  for (const r of add.resources) resources.set(r.id, r);
  const relationships = new Map(base.relationships.map((r) => [r.id, r]));
  for (const r of add.relationships) relationships.set(r.id, r);
  return {
    meta: {
      ...add.meta,
      partial: true,
      degree: { ...base.meta.degree, ...add.meta.degree },
      owned: { ...base.meta.owned, ...add.meta.owned },
    },
    resources: [...resources.values()],
    relationships: [...relationships.values()],
    vocabulary: add.vocabulary ?? base.vocabulary,
  };
}

/** An empty partial graph: a large inventory before anything is loaded. */
export function emptyPartial(meta: CanonicalGraph['meta']): CanonicalGraph {
  return { meta: { ...meta, partial: true, degree: {} }, resources: [], relationships: [] };
}

/** Relationships of `id` whose other end is not loaded yet (0 for a full graph). */
export function notLoadedRelationships(index: GraphIndex, id: ResourceId): number {
  const degree = index.graph.meta.degree?.[id];
  if (degree === undefined) return 0;
  return Math.max(0, degree - index.out(id).length - index.in(id).length);
}

const loadedOwned = new WeakMap<GraphIndex, Map<ResourceId, number>>();

/** Resources an owner owns that are not loaded yet (0 for non-owners and full graphs). */
export function notLoadedOwned(index: GraphIndex, id: ResourceId): number {
  const total = index.graph.meta.owned?.[id];
  if (!total) return 0;
  let counts = loadedOwned.get(index);
  if (!counts) {
    counts = new Map();
    for (const r of index.graph.resources) {
      if (!r.account) continue;
      const type = index.vocab.ownerType(r.provider);
      const owner = type ? index.byNative(type, r.account) : undefined;
      if (owner && owner.id !== r.id) counts.set(owner.id, (counts.get(owner.id) ?? 0) + 1);
    }
    loadedOwned.set(index, counts);
  }
  return Math.max(0, total - (counts.get(id) ?? 0));
}

/** Everything about `id` not loaded yet: its relationships, and for an owner what it owns. */
export function notLoaded(index: GraphIndex, id: ResourceId): number {
  return notLoadedRelationships(index, id) + notLoadedOwned(index, id);
}
