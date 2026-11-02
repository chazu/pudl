// Visual ID conventions and node sizes, shared by the node and edge builders.

import type { ResourceId } from '../model/canonical';

export const SIZES = {
  leaf: { width: 184, height: 44 },
  collapsed: { width: 204, height: 56 },
  unresolved: { width: 184, height: 40 },
  port: 8,
  semanticPort: 10,
};

/** Leaf nodes are `n:<resourceId>`; groups reuse their semantic group ID. */
export function nodeIdForResource(id: ResourceId): string {
  return `n:${id}`;
}
