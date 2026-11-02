// ELK adapter: the only module that knows ELK's JSON graph format.
//
// Layered layouts use INCLUDE_CHILDREN so edges between nodes in different
// nested groups are routed in one pass, with orthogonal routing and fixed
// port sides. Coordinates come back absolute (elk.json.*Coords = ROOT).
//
// Stability: ELK is deterministic, so children are always fed in the
// projection's canonical order and ELK is told to respect it
// (considerModelOrder). Identical state then reproduces the identical
// layout, and a small change perturbs only what it touches. Reordering
// children by their previous on-screen positions was measured to be worse
// (it changes the model order and so the whole layout); see
// docs/frontend.md#layout-stability. ELK's interactive layering and
// crossing minimisation are not supported with INCLUDE_CHILDREN.

import ELK from 'elkjs/lib/elk.bundled.js';
import type { ElkExtendedEdge, ElkNode, ElkPort, LayoutOptions } from 'elkjs/lib/elk-api';
import type { VisualGraph, VisualNode } from '../visual/types';
import { SIZES } from '../visual/ids';
import { ISOLATED_PREFIX, isolatedPlan, packedIds, placeBlock, type IsolatedBlock } from './isolatedPacking';
import type { Box, EdgeRoute, LayoutConfig, LayoutEngine, LayoutHints, LayoutResult, Point } from './types';

const GROUP_PADDING = '[top=34,left=12,bottom=12,right=12]';

function rootOptions(config: LayoutConfig): LayoutOptions {
  const common: LayoutOptions = {
    'elk.json.shapeCoords': 'ROOT',
    'elk.json.edgeCoords': 'ROOT',
    'elk.padding': '[top=16,left=16,bottom=16,right=16]',
  };
  if (config.algorithm === 'packed') {
    return { ...common, 'elk.algorithm': 'rectpacking', 'elk.spacing.nodeNode': '14', 'elk.aspectRatio': '1.8' };
  }
  return {
    ...common,
    'elk.algorithm': 'layered',
    'elk.direction': config.direction,
    'elk.hierarchyHandling': 'INCLUDE_CHILDREN',
    'elk.edgeRouting': 'ORTHOGONAL',
    'elk.layered.considerModelOrder.strategy': 'NODES_AND_EDGES',
    // NETWORK_SIMPLEX throws on some cross-hierarchy edges with
    // INCLUDE_CHILDREN ("NEdge must have a source and target"); BK does not.
    'elk.layered.nodePlacement.strategy': 'BRANDES_KOEPF',
    'elk.layered.nodePlacement.bk.fixedAlignment': 'BALANCED',
    'elk.layered.mergeEdges': 'false',
    'elk.spacing.nodeNode': '18',
    'elk.layered.spacing.nodeNodeBetweenLayers': '40',
    'elk.spacing.edgeNode': '14',
    'elk.spacing.edgeEdge': '7',
    'elk.layered.spacing.edgeNodeBetweenLayers': '14',
    'elk.layered.spacing.edgeEdgeBetweenLayers': '7',
    'elk.spacing.componentComponent': '28',
    'elk.edgeLabels.inline': 'true',
  };
}

/** Canonical child order: the projection's order, independent of history. */
export function stableOrder(nodes: VisualNode[]): VisualNode[] {
  return [...nodes].sort((a, b) => a.order - b.order);
}

export function toElkGraph(graph: VisualGraph, config: LayoutConfig): ElkNode {
  const layered = config.algorithm === 'layered';
  const plan = layered ? isolatedPlan(graph) : new Map<string, IsolatedBlock>();
  const packed = packedIds(plan);
  const placeholder = (parentId: string | null): ElkNode[] => {
    const block = plan.get(`${ISOLATED_PREFIX}${parentId ?? '__root__'}`);
    return block ? [{ id: block.id, width: block.width, height: block.height }] : [];
  };

  const toNode = (n: VisualNode): ElkNode => {
    const isGroup = n.kind === 'group';
    const ports: ElkPort[] = layered
      ? n.ports.map((p, i) => {
          const size = p.kind === 'semantic' ? SIZES.semanticPort : SIZES.port;
          return {
            id: p.id,
            width: size,
            height: size,
            layoutOptions: { 'elk.port.side': p.side, 'elk.port.index': String(i) },
          };
        })
      : [];
    const options: LayoutOptions = { 'elk.portConstraints': 'FIXED_SIDE' };
    if (isGroup) {
      options['elk.padding'] = GROUP_PADDING;
      options['elk.nodeSize.constraints'] = 'MINIMUM_SIZE';
      options['elk.nodeSize.minimum'] = '(180,60)';
      if (!layered) {
        options['elk.algorithm'] = 'rectpacking';
        options['elk.spacing.nodeNode'] = '10';
        options['elk.aspectRatio'] = '1.8';
      }
    }
    const children = [
      ...stableOrder(n.children.map((c) => graph.nodes.get(c)!).filter((c) => c && !packed.has(c.id))).map(toNode),
      ...placeholder(n.id),
    ];
    return {
      id: n.id,
      ...(isGroup ? {} : { width: n.width, height: n.height }),
      ports,
      children,
      layoutOptions: options,
    };
  };

  const roots = [
    ...stableOrder(graph.roots.map((id) => graph.nodes.get(id)!).filter((n) => !packed.has(n.id))).map(toNode),
    ...placeholder(null),
  ];
  const edges: ElkExtendedEdge[] = layered
    ? graph.edges.map((e) => ({
        id: e.id,
        sources: [e.sourcePort],
        targets: [e.targetPort],
        labels: e.aggregated ? [{ id: `${e.id}:label`, text: e.label, width: e.label.length * 6.2 + 12, height: 16 }] : [],
      }))
    : [];
  return { id: '__root__', layoutOptions: rootOptions(config), children: roots, edges };
}

export function fromElkGraph(out: ElkNode, graph: VisualGraph, durationMs: number, config?: LayoutConfig): LayoutResult {
  const nodes = new Map<string, Box>();
  const ports = new Map<string, Box>();
  const plan = !config || config.algorithm === 'layered' ? isolatedPlan(graph) : new Map<string, IsolatedBlock>();
  const walk = (n: ElkNode) => {
    for (const c of n.children ?? []) {
      const block = plan.get(c.id);
      if (block) {
        const at = { x: c.x ?? 0, y: c.y ?? 0, width: c.width ?? 0, height: c.height ?? 0 };
        for (const [id, box] of placeBlock(block, at)) nodes.set(id, box);
        continue;
      }
      nodes.set(c.id, { x: c.x ?? 0, y: c.y ?? 0, width: c.width ?? 0, height: c.height ?? 0 });
      for (const p of c.ports ?? []) {
        ports.set(p.id, { x: p.x ?? 0, y: p.y ?? 0, width: p.width ?? 0, height: p.height ?? 0 });
      }
      walk(c);
    }
  };
  walk(out);

  const edges = new Map<string, EdgeRoute>();
  const routed = new Map((out.edges ?? []).map((e) => [e.id, e]));
  for (const e of graph.edges) {
    const r = routed.get(e.id);
    const sections: Point[][] = (r?.sections ?? []).map((s) => [s.startPoint, ...(s.bendPoints ?? []), s.endPoint]);
    const label = r?.labels?.[0];
    if (sections.length > 0) {
      edges.set(e.id, {
        sections,
        routed: true,
        labelAt: label && label.x !== undefined ? { x: label.x + (label.width ?? 0) / 2, y: (label.y ?? 0) + (label.height ?? 0) / 2 } : undefined,
      });
    } else {
      edges.set(e.id, { sections: [straight(nodes.get(e.source), nodes.get(e.target))], routed: false });
    }
  }
  return {
    nodes,
    ports,
    edges,
    bounds: { x: 0, y: 0, width: out.width ?? 0, height: out.height ?? 0 },
    durationMs,
  };
}

function straight(a?: Box, b?: Box): Point[] {
  const c = (box?: Box): Point => (box ? { x: box.x + box.width / 2, y: box.y + box.height / 2 } : { x: 0, y: 0 });
  return [c(a), c(b)];
}

export class ElkLayoutEngine implements LayoutEngine {
  readonly name = 'elk';
  private readonly elk = new ELK();

  /** `hints` is accepted for the interface; this engine relies on deterministic model order instead. */
  async layout(graph: VisualGraph, config: LayoutConfig, _hints: LayoutHints): Promise<LayoutResult> {
    const t0 = performance.now();
    const out = await this.elk.layout(toElkGraph(graph, config));
    return fromElkGraph(out, graph, performance.now() - t0, config);
  }
}
