// Layout state that outlives a single layout run.
//
// Remembers the last result per view (projection + direction) so the next
// layout can keep ordering and positions, and so switching back to a view
// does not reshuffle it. This is the seam for better incremental layout
// later (e.g. pinning, partial relayout of only the changed subtree).

import type { LayoutResult } from './types';

export class LayoutMemory {
  private readonly results = new Map<string, LayoutResult>();

  previous(viewKey: string): LayoutResult | undefined {
    return this.results.get(viewKey);
  }

  remember(viewKey: string, result: LayoutResult): void {
    this.results.set(viewKey, result);
  }

  forget(viewKey: string): void {
    this.results.delete(viewKey);
  }
}

/**
 * Average displacement of nodes present in both layouts, relative to an
 * anchor node (so a whole-diagram shift does not count). Used by tests and
 * the status bar to quantify stability.
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
