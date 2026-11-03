// Partial graphs: merge neighbourhoods loaded on demand, and count what is
// not loaded yet. A relationship to an unloaded resource is never in the
// graph, so it can never be mistaken for one to an uncollected resource.

import type { CanonicalGraph, GraphIndex, ResourceId } from '../model/canonical';

/** Union of two graphs by ID; `add` wins on conflicts and supplies the meta and vocabulary. */
export function mergeGraphs(base: CanonicalGraph, add: CanonicalGraph): CanonicalGraph {
  const resources = new Map(base.resources.map((r) => [r.id, r]));
  for (const r of add.resources) resources.set(r.id, r);
  const relationships = new Map(base.relationships.map((r) => [r.id, r]));
  for (const r of add.relationships) relationships.set(r.id, r);
  return {
    meta: { ...add.meta, partial: true, degree: { ...base.meta.degree, ...add.meta.degree } },
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
export function notLoaded(index: GraphIndex, id: ResourceId): number {
  const degree = index.graph.meta.degree?.[id];
  if (degree === undefined) return 0;
  return Math.max(0, degree - index.out(id).length - index.in(id).length);
}
