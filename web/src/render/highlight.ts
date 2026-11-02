// Selection → what to emphasise on the canvas. Highlighting follows drawn
// edges only; it never implies connections the visual graph doesn't show.

import type { ResourceId } from '../model/canonical';
import type { VisualGraph } from '../visual/types';

export type Selection =
  | { kind: 'resource'; id: ResourceId }
  | { kind: 'group'; id: string }
  | { kind: 'edge'; id: string }
  | null;

export interface Highlight {
  active: boolean;
  nodes: Set<string>;
  edges: Set<string>;
  primaryNode?: string;
  primaryEdge?: string;
}

export function computeHighlight(graph: VisualGraph, sel: Selection): Highlight {
  const empty: Highlight = { active: false, nodes: new Set(), edges: new Set() };
  if (!sel) return empty;
  if (sel.kind === 'edge') {
    const e = graph.edges.find((x) => x.id === sel.id);
    if (!e) return empty;
    return { active: true, nodes: new Set([e.source, e.target]), edges: new Set([e.id]), primaryEdge: e.id };
  }
  const nodeId = sel.kind === 'group' ? sel.id : graph.resourceToNode.get(sel.id);
  if (!nodeId || !graph.nodes.has(nodeId)) return empty;
  const nodes = new Set([nodeId]);
  const edges = new Set<string>();
  for (const e of graph.edges) {
    if (e.source === nodeId || e.target === nodeId) {
      edges.add(e.id);
      nodes.add(e.source);
      nodes.add(e.target);
    }
  }
  return { active: true, nodes, edges, primaryNode: nodeId };
}
