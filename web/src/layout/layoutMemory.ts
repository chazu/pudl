// Layout state that outlives a single layout run, per view (projection +
// direction):
//   - the last result, passed to the engine as LayoutHints.previous so the
//     incremental engine keeps the existing relative order;
//   - results by graph signature, so returning to a state already laid out
//     (collapse then expand, a filter toggled back) reproduces it exactly.
// Relayout forgets both.

import type { VisualGraph } from '../visual/types';
import type { LayoutConfig, LayoutResult } from './types';

const MAX_CACHED = 24;

interface ViewMemory {
  last?: LayoutResult;
  bySignature: Map<string, LayoutResult>;
}

export class LayoutMemory {
  private readonly views = new Map<string, ViewMemory>();

  private view(viewKey: string): ViewMemory {
    let v = this.views.get(viewKey);
    if (!v) {
      v = { bySignature: new Map() };
      this.views.set(viewKey, v);
    }
    return v;
  }

  previous(viewKey: string): LayoutResult | undefined {
    return this.views.get(viewKey)?.last;
  }

  cached(viewKey: string, signature: string): LayoutResult | undefined {
    return this.views.get(viewKey)?.bySignature.get(signature);
  }

  remember(viewKey: string, result: LayoutResult, signature?: string): void {
    const v = this.view(viewKey);
    v.last = result;
    if (signature) {
      v.bySignature.delete(signature);
      v.bySignature.set(signature, result);
      if (v.bySignature.size > MAX_CACHED) v.bySignature.delete(v.bySignature.keys().next().value!);
    }
  }

  forget(viewKey: string): void {
    this.views.delete(viewKey);
  }
}

/**
 * Everything about a visual graph that a layout depends on: nodes (parent,
 * kind, size, ports), edges (endpoints, ports, label) and the config. Equal
 * signatures lay out identically.
 */
export function layoutSignature(graph: VisualGraph, config: LayoutConfig): string {
  const parts: string[] = [config.algorithm, config.direction];
  for (const n of [...graph.nodes.values()].sort((a, b) => (a.id < b.id ? -1 : 1))) {
    parts.push(`${n.id}|${n.parentId}|${n.kind}|${n.width}x${n.height}|${n.order}|${n.ports.map((p) => `${p.id}:${p.side}`).join(',')}`);
  }
  for (const e of [...graph.edges].sort((a, b) => (a.id < b.id ? -1 : 1))) {
    parts.push(`${e.id}|${e.sourcePort}|${e.targetPort}|${e.aggregated ? e.label : ''}`);
  }
  // FNV-1a, 64-bit via two 32-bit halves: compact and stable.
  let h1 = 0x811c9dc5;
  let h2 = 0x01000193;
  for (const s of parts) {
    for (let i = 0; i < s.length; i++) {
      const c = s.charCodeAt(i);
      h1 = Math.imul(h1 ^ c, 0x01000193);
      h2 = Math.imul(h2 ^ c, 0x811c9dc5);
    }
    h1 = Math.imul(h1 ^ 10, 0x01000193);
  }
  return `${(h1 >>> 0).toString(16)}${(h2 >>> 0).toString(16)}:${parts.length}`;
}

/**
 * Average displacement of nodes present in both layouts, relative to an
 * anchor node (so a whole-diagram shift does not count).
 */
export function displacement(a: LayoutResult, b: LayoutResult, anchorId?: string): number {
  const ax = anchorId ? a.nodes.get(anchorId) : undefined;
  const bx = anchorId ? b.nodes.get(anchorId) : undefined;
  const dx0 = ax && bx ? bx.x - ax.x : 0;
  const dy0 = ax && bx ? bx.y - ax.y : 0;
  let sum = 0;
  let n = 0;
  for (const [id, pa] of a.nodes) {
    const pb = b.nodes.get(id);
    if (!pb) continue;
    sum += Math.hypot(pb.x - pa.x - dx0, pb.y - pa.y - dy0);
    n++;
  }
  return n === 0 ? 0 : sum / n;
}
