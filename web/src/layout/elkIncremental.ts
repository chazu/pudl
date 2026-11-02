// Incremental ELK layout: each group is laid out on its own
// (hierarchyHandling SEPARATE_CHILDREN), and an edge that crosses group
// boundaries is split into segments through generated boundary ports, one
// segment per level. Because every level is then a flat graph, ELK accepts
// its interactive strategies there: given the previous layout, layering,
// crossing minimisation and cycle breaking keep the old relative order, so
// collapsing a group or adding a resource does not reshuffle the rest.
// Node placement stays normal (interactive placement drifts and grows).
// Without a previous layout this is an ordinary separate-children layout.

import ELK from 'elkjs/lib/elk.bundled.js';
import type { ElkExtendedEdge, ElkNode, ElkPort, LayoutOptions } from 'elkjs/lib/elk-api';
import type { PortSide, VisualGraph, VisualNode } from '../visual/types';
import { SIZES } from '../visual/ids';
import { GROUP_PADDING, rootOptions, stableOrder, straight } from './elkAdapter';
import { ISOLATED_PREFIX, isolatedPlan, packedIds, placeBlock } from './isolatedPacking';
import type { Box, EdgeRoute, LayoutConfig, LayoutEngine, LayoutHints, LayoutResult, Point } from './types';

const ROOT = '__root__';
const BOUNDARY_PORT = 4;
const INTERACTIVE: LayoutOptions = {
  'elk.layered.layering.strategy': 'INTERACTIVE',
  'elk.layered.crossingMinimization.strategy': 'INTERACTIVE',
  'elk.layered.cycleBreaking.strategy': 'INTERACTIVE',
};

interface Plan {
  root: ElkNode;
  /** Visual edge -> its segment edge IDs, source end first. */
  segments: Map<string, string[]>;
  /** Visual edge -> the segment carrying its label (the one at the common ancestor). */
  labelSegment: Map<string, string>;
}

function levelOptions(config: LayoutConfig): LayoutOptions {
  const o = { ...rootOptions(config) };
  delete o['elk.hierarchyHandling'];
  return { ...o, 'elk.hierarchyHandling': 'SEPARATE_CHILDREN' };
}

export function toElkIncremental(graph: VisualGraph, config: LayoutConfig, hints: LayoutHints): Plan {
  const rightward = config.direction === 'RIGHT';
  const outSide: PortSide = rightward ? 'EAST' : 'SOUTH';
  const inSide: PortSide = rightward ? 'WEST' : 'NORTH';
  const prev = hints.previous?.nodes;
  const plan = isolatedPlan(graph);
  const packed = packedIds(plan);

  // ELK nodes for every visual node, plus the root.
  const elk = new Map<string, ElkNode>();
  const base = levelOptions(config);
  elk.set(ROOT, { id: ROOT, layoutOptions: { ...base }, children: [], edges: [] });
  for (const n of graph.nodes.values()) {
    if (packed.has(n.id)) continue;
    const ports: ElkPort[] = n.ports.map((p, i) => {
      const size = p.kind === 'semantic' ? SIZES.semanticPort : SIZES.port;
      return { id: p.id, width: size, height: size, layoutOptions: { 'elk.port.side': p.side, 'elk.port.index': String(i) } };
    });
    const group = n.kind === 'group';
    const options: LayoutOptions = group
      ? { ...base, 'elk.padding': GROUP_PADDING, 'elk.nodeSize.constraints': 'MINIMUM_SIZE', 'elk.nodeSize.minimum': '(180,60)', 'elk.portConstraints': 'FIXED_SIDE' }
      : { 'elk.portConstraints': 'FIXED_SIDE' };
    elk.set(n.id, { id: n.id, ...(group ? {} : { width: n.width, height: n.height }), ports, children: [], edges: [], layoutOptions: options });
  }
  const container = (id: string | null) => elk.get(id ?? ROOT)!;
  const parentOf = (id: string): string | null => graph.nodes.get(id)?.parentId ?? null;
  const ancestors = (id: string): (string | null)[] => {
    const out: (string | null)[] = [];
    for (let c = parentOf(id); ; c = parentOf(c)) {
      out.push(c);
      if (c === null) return out;
    }
  };

  // Children in model order; one placeholder per packed block.
  const attach = (parentId: string | null, kids: VisualNode[]) => {
    const host = container(parentId);
    for (const k of stableOrder(kids.filter((k) => !packed.has(k.id)))) host.children!.push(elk.get(k.id)!);
    const block = plan.get(`${ISOLATED_PREFIX}${parentId ?? ROOT}`);
    if (block) host.children!.push({ id: block.id, width: block.width, height: block.height });
  };
  attach(null, graph.roots.map((id) => graph.nodes.get(id)!));
  for (const n of graph.nodes.values()) if (n.kind === 'group') attach(n.id, n.children.map((c) => graph.nodes.get(c)!));

  // Split every edge at each group boundary it crosses.
  const segments = new Map<string, string[]>();
  const labelSegment = new Map<string, string>();
  for (const e of graph.edges) {
    const up = ancestors(e.source);
    const down = ancestors(e.target);
    const lca = up.find((c) => down.includes(c)) ?? null;
    const ids: string[] = [];
    const add = (holder: string | null, from: string, to: string, label?: boolean) => {
      const id = `${e.id}#${ids.length}`;
      ids.push(id);
      const edge: ElkExtendedEdge = { id, sources: [from], targets: [to] };
      if (label && e.aggregated) edge.labels = [{ id: `${id}:label`, text: e.label, width: e.label.length * 6.2 + 12, height: 16 }];
      container(holder).edges!.push(edge);
      if (label) labelSegment.set(e.id, id);
    };
    const boundary = (group: string, side: PortSide, tag: string) => {
      const id = `${e.id}@${group}:${tag}`;
      container(group).ports!.push({ id, width: BOUNDARY_PORT, height: BOUNDARY_PORT, layoutOptions: { 'elk.port.side': side } });
      return id;
    };
    let from = e.sourcePort;
    for (const g of up.slice(0, up.indexOf(lca))) {
      const port = boundary(g!, outSide, 'out');
      add(g, from, port);
      from = port;
    }
    const inner = down.slice(0, down.indexOf(lca));
    let to = e.targetPort;
    const downs: [string, string, string][] = [];
    for (const g of inner) {
      const port = boundary(g!, inSide, 'in');
      downs.push([g!, port, to]);
      to = port;
    }
    add(lca, from, to, true);
    for (const [g, port, next] of downs.reverse()) add(g, port, next);
    segments.set(e.id, ids);
  }

  // Interactive levels: keep the previous relative order where most children were laid out before.
  if (prev) {
    for (const [hostId, host] of elk) {
      const kids = (host.children ?? []).filter((k) => !k.id.startsWith(ISOLATED_PREFIX));
      if (kids.length < 2) continue;
      const origin = hostId === ROOT ? { x: 0, y: 0 } : prev.get(hostId);
      const known = kids.filter((k) => prev.has(k.id));
      if (!origin || known.length * 2 < kids.length) continue;
      host.layoutOptions = { ...host.layoutOptions, ...INTERACTIVE };
      let farthest = 0;
      // Interactive strategies order by position. A node that changed size
      // (a group collapsed or expanded) keeps its previous centre, not its
      // corner, so it stays among the same neighbours.
      for (const k of known) {
        const p = prev.get(k.id)!;
        k.x = p.x + p.width / 2 - (k.width ?? p.width) / 2 - origin.x;
        k.y = p.y + p.height / 2 - (k.height ?? p.height) / 2 - origin.y;
        farthest = Math.max(farthest, rightward ? k.x : k.y);
      }
      // New children: after everything else along the flow, so they get their own layer.
      let i = 0;
      for (const k of kids) {
        if (prev.has(k.id)) continue;
        const along = farthest + 60 + i * 2;
        k.x = rightward ? along : i * 60;
        k.y = rightward ? i * 60 : along;
        i++;
      }
    }
  }
  return { root: container(null), segments, labelSegment };
}

export function fromElkIncremental(out: ElkNode, graph: VisualGraph, plan: Plan, durationMs: number): LayoutResult {
  const nodes = new Map<string, Box>();
  const ports = new Map<string, Box>();
  const edges = new Map<string, ElkExtendedEdge>();
  const blocks = isolatedPlan(graph);
  const walk = (n: ElkNode) => {
    for (const e of n.edges ?? []) edges.set(e.id, e);
    for (const c of n.children ?? []) {
      const block = blocks.get(c.id);
      const box = { x: c.x ?? 0, y: c.y ?? 0, width: c.width ?? 0, height: c.height ?? 0 };
      if (block) {
        for (const [id, b] of placeBlock(block, box)) nodes.set(id, b);
        continue;
      }
      nodes.set(c.id, box);
      for (const p of c.ports ?? []) ports.set(p.id, { x: p.x ?? 0, y: p.y ?? 0, width: p.width ?? 0, height: p.height ?? 0 });
      walk(c);
    }
  };
  walk(out);

  const routes = new Map<string, EdgeRoute>();
  for (const e of graph.edges) {
    const sections: Point[][] = [];
    for (const id of plan.segments.get(e.id) ?? []) {
      for (const s of edges.get(id)?.sections ?? []) sections.push([s.startPoint, ...(s.bendPoints ?? []), s.endPoint]);
    }
    const label = edges.get(plan.labelSegment.get(e.id) ?? '')?.labels?.[0];
    routes.set(
      e.id,
      sections.length > 0
        ? {
            sections,
            routed: true,
            labelAt: label && label.x !== undefined ? { x: label.x + (label.width ?? 0) / 2, y: (label.y ?? 0) + (label.height ?? 0) / 2 } : undefined,
          }
        : { sections: [straight(nodes.get(e.source), nodes.get(e.target))], routed: false },
    );
  }
  return { nodes, ports, edges: routes, bounds: { x: 0, y: 0, width: out.width ?? 0, height: out.height ?? 0 }, durationMs };
}

export class ElkIncrementalEngine implements LayoutEngine {
  readonly name = 'elk-incremental';
  private readonly elk = new ELK();

  async layout(graph: VisualGraph, config: LayoutConfig, hints: LayoutHints): Promise<LayoutResult> {
    const t0 = performance.now();
    const plan = toElkIncremental(graph, config, hints);
    const out = await this.elk.layout(plan.root);
    return fromElkIncremental(out, graph, plan, performance.now() - t0);
  }
}
