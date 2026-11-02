// Unconnected leaves. ELK's layered algorithm puts every node without edges
// in the same layer, so a group with many of them becomes one long row (the
// security view's IAM users, a VPC's unused security groups). They have no
// edges to route, so the adapter takes them out of the ELK graph, reserves
// one placeholder box per parent sized for a compact grid, and places them in
// that grid afterwards. The plan is a pure function of the visual graph, so
// the adapter computes it again when reading ELK's result.

import type { VisualGraph, VisualNode } from '../visual/types';
import type { Box } from './types';

export const ISOLATED_PREFIX = '__isolated:';
const GAP_X = 18;
const GAP_Y = 14;
/** Target width:height of a grid. */
const ASPECT = 1.6;
/** Fewer unconnected siblings than this stay in the layered layout. */
const MIN_TO_PACK = 3;

export interface IsolatedBlock {
  id: string;
  parentId: string | null;
  members: VisualNode[];
  cols: number;
  width: number;
  height: number;
}

export function isolatedPlan(graph: VisualGraph): Map<string, IsolatedBlock> {
  const touched = new Set<string>();
  for (const e of graph.edges) {
    touched.add(e.source);
    touched.add(e.target);
  }
  const byParent = new Map<string | null, VisualNode[]>();
  for (const n of graph.nodes.values()) {
    if (n.kind === 'group' || touched.has(n.id) || n.ports.length > 0) continue;
    const list = byParent.get(n.parentId) ?? [];
    list.push(n);
    byParent.set(n.parentId, list);
  }
  const plan = new Map<string, IsolatedBlock>();
  for (const [parentId, members] of byParent) {
    if (members.length < MIN_TO_PACK) continue;
    members.sort((a, b) => a.order - b.order);
    const cellW = Math.max(...members.map((m) => m.width)) + GAP_X;
    const cellH = Math.max(...members.map((m) => m.height)) + GAP_Y;
    const cols = Math.max(1, Math.min(members.length, Math.round(Math.sqrt((members.length * ASPECT * cellH) / cellW))));
    const rows = Math.ceil(members.length / cols);
    const id = `${ISOLATED_PREFIX}${parentId ?? '__root__'}`;
    plan.set(id, { id, parentId, members, cols, width: cols * cellW - GAP_X, height: rows * cellH - GAP_Y });
  }
  return plan;
}

/** IDs of the nodes a plan takes out of the ELK graph. */
export function packedIds(plan: Map<string, IsolatedBlock>): Set<string> {
  const out = new Set<string>();
  for (const block of plan.values()) for (const m of block.members) out.add(m.id);
  return out;
}

/** Member boxes inside a placed block, row by row in model order. */
export function placeBlock(block: IsolatedBlock, at: Box): Map<string, Box> {
  const out = new Map<string, Box>();
  const cellW = (block.width + GAP_X) / block.cols;
  const cellH = Math.max(...block.members.map((m) => m.height)) + GAP_Y;
  block.members.forEach((m, i) => {
    const col = i % block.cols;
    const row = Math.floor(i / block.cols);
    out.set(m.id, { x: at.x + col * cellW, y: at.y + row * cellH, width: m.width, height: m.height });
  });
  return out;
}
