import { beforeAll, describe, expect, it } from 'vitest';
import { buildDemoGraph } from '../data/mock';
import { GraphIndex } from '../model/canonical';
import { project } from '../projection/project';
import { PROJECTIONS, projectionById } from '../projection/registry';
import type { ProjectionId } from '../projection/types';
import { buildVisualGraph } from '../visual/build';
import type { VisualGraph } from '../visual/types';
import { initialViewState, type ViewState } from '../visual/viewState';
import { ElkLayoutEngine } from './elkAdapter';
import { ElkIncrementalEngine } from './elkIncremental';
import { layoutSignature } from './layoutMemory';
import type { Box, LayoutEngine, LayoutHints, LayoutResult } from './types';

const index = new GraphIndex(buildDemoGraph());
let engine: LayoutEngine = new ElkLayoutEngine();

async function layoutOf(
  id: ProjectionId,
  tweak: (s: ViewState) => ViewState = (s) => s,
  hints: LayoutHints = {},
  using: LayoutEngine = engine,
) {
  const p = projectionById(id);
  const view = project(index, p);
  const state = tweak(initialViewState(view));
  const graph = buildVisualGraph(view, index, state);
  const result = await using.layout(graph, { ...p.layout, direction: state.direction }, hints);
  return { graph, result, view };
}

const inside = (child: Box, parent: Box) =>
  child.x >= parent.x - 0.5 &&
  child.y >= parent.y - 0.5 &&
  child.x + child.width <= parent.x + parent.width + 0.5 &&
  child.y + child.height <= parent.y + parent.height + 0.5;

/** Share of sibling pairs whose relative placement (left/right, above/below) is unchanged. */
function siblingOrderKept(graph: VisualGraph, a: LayoutResult, b: LayoutResult): number {
  let kept = 0;
  let total = 0;
  const sign = (v: number) => Math.sign(Math.round(v / 4));
  for (const n of graph.nodes.values()) {
    const siblings = n.parentId ? graph.nodes.get(n.parentId)!.children : graph.roots;
    for (const m of siblings) {
      if (m <= n.id) continue;
      const [pa, qa, pb, qb] = [a.nodes.get(n.id), a.nodes.get(m), b.nodes.get(n.id), b.nodes.get(m)];
      if (!pa || !qa || !pb || !qb) continue;
      total++;
      if (sign(qa.x - pa.x) === sign(qb.x - pb.x) && sign(qa.y - pa.y) === sign(qb.y - pb.y)) kept++;
    }
  }
  return total === 0 ? 1 : kept / total;
}

for (const [name, make] of [['classic', () => new ElkLayoutEngine()], ['incremental', () => new ElkIncrementalEngine()]] as const) {
describe(`ELK ${name} engine`, () => {
  beforeAll(() => {
    engine = make();
  });

  for (const p of PROJECTIONS.filter((p) => name === 'classic' || p.layout.algorithm === 'layered')) {
    it(`lays out the ${p.id} projection with nested boxes and routed edges`, async () => {
      const { graph, result } = await layoutOf(p.id);
      for (const n of graph.nodes.values()) {
        const box = result.nodes.get(n.id);
        expect(box, n.id).toBeDefined();
        if (n.parentId) expect(inside(box!, result.nodes.get(n.parentId)!), `${n.id} inside ${n.parentId}`).toBe(true);
      }
      for (const e of graph.edges) {
        const route = result.edges.get(e.id)!;
        expect(route.routed).toBe(p.layout.algorithm === 'layered');
        expect(route.sections.length).toBeGreaterThan(0);
      }
    });
  }

  it('routes edges orthogonally and starts them at their ports', async () => {
    const { graph, result } = await layoutOf('network');
    for (const e of graph.edges) {
      const route = result.edges.get(e.id)!;
      for (const pts of route.sections) {
        for (let i = 1; i < pts.length; i++) {
          const horizontal = Math.abs(pts[i].y - pts[i - 1].y) < 0.5;
          const vertical = Math.abs(pts[i].x - pts[i - 1].x) < 0.5;
          expect(horizontal || vertical).toBe(true);
        }
      }
      const port = result.ports.get(e.sourcePort)!;
      const start = route.sections[0][0];
      expect(start.x).toBeGreaterThanOrEqual(port.x - 0.5);
      expect(start.x).toBeLessThanOrEqual(port.x + port.width + 0.5);
      expect(start.y).toBeGreaterThanOrEqual(port.y - 0.5);
      expect(start.y).toBeLessThanOrEqual(port.y + port.height + 0.5);
    }
  });

  it('is deterministic: the same state reproduces the same layout', async () => {
    const a = await layoutOf('network');
    const b = await layoutOf('network');
    expect([...b.result.nodes]).toEqual([...a.result.nodes]);
  });

  // Filters do not reach layout by default (see visual/build.test.ts, "render-only
  // filters"); collapse is the structural change that does, so measure it.
});
}

describe('ELK classic engine stability', () => {
  beforeAll(() => {
    engine = new ElkLayoutEngine();
  });

  it('keeps most sibling ordering when a group collapses', async () => {
    const cases: [ProjectionId, (s: ViewState) => ViewState, number][] = [
      ['network', (s) => ({ ...s, collapsed: new Set(['g:aws:vpc-0prodcore']) }), 0.7],
      ['network', (s) => ({ ...s, collapsed: new Set(['g:aws:vpc-0shared']) }), 0.7],
      ['security', (s) => ({ ...s, collapsed: new Set(['g:aws:account:222222222222']) }), 0.9],
      ['application', (s) => ({ ...s, collapsed: new Set(['g:logical:svc:orders-api']) }), 0.75],
    ];
    for (const [id, tweak, threshold] of cases) {
      const base = await layoutOf(id);
      const changed = await layoutOf(id, tweak);
      expect(siblingOrderKept(base.graph, base.result, changed.result), id).toBeGreaterThanOrEqual(threshold);
    }
  });
});

/** The group holding the most members: collapsing it is the worst case. */
function largestGroup(view: ReturnType<typeof project>): string {
  const size = (gid: string) =>
    [...view.members.values()].filter((m) => {
      for (let c = m.parentId; c; c = view.groups.get(c)?.parentId ?? null) if (c === gid) return true;
      return false;
    }).length;
  return [...view.groups.values()].sort((a, b) => size(b.id) - size(a.id) || a.order - b.order)[0].id;
}

describe('ELK incremental engine stability', () => {
  const inc = new ElkIncrementalEngine();

  // Exact reproduction of a state seen before is the layout cache's job
  // (layoutMemory); relaying out from its own result keeps nearly all order.
  it('keeps nearly all sibling order when relaid out from its own previous result', async () => {
    for (const id of ['network', 'security', 'application'] as const) {
      const a = await layoutOf(id, (s) => s, {}, inc);
      const b = await layoutOf(id, (s) => s, { previous: a.result }, inc);
      expect(siblingOrderKept(a.graph, a.result, b.result), id).toBeGreaterThanOrEqual(0.95);
    }
  });

  // Collapsing the largest group, and a refresh that drops a resource, are
  // the worst cases for stability. Across them the incremental engine must
  // keep more sibling order than the single-pass engine overall, and never
  // collapse badly in one case. (Measured: network 0.85 vs 0.73, security
  // 0.89 vs 0.39; the application view, with three top-level groups, can
  // trade one vertical flip.)
  it('keeps more sibling order than the single-pass engine across collapse and refresh', async () => {
    const classic = new ElkLayoutEngine();
    const first = buildDemoGraph().resources[0].id;
    const g = buildDemoGraph();
    const refreshed = new GraphIndex({
      ...g,
      resources: g.resources.filter((r) => r.id !== first),
      relationships: g.relationships.filter((r) => r.source !== first && r.target !== first),
    });
    const keptBy = async (using: LayoutEngine, id: ProjectionId, change: 'collapse' | 'refresh') => {
      const base = await layoutOf(id, (s) => s, {}, using);
      const p = projectionById(id);
      let changed: { graph: VisualGraph; result: LayoutResult };
      if (change === 'collapse') {
        changed = await layoutOf(id, (s) => ({ ...s, collapsed: new Set([largestGroup(base.view)]) }), { previous: base.result }, using);
      } else {
        const view = project(refreshed, p);
        const state = initialViewState(view);
        const graph = buildVisualGraph(view, refreshed, state);
        changed = { graph, result: await using.layout(graph, { ...p.layout, direction: state.direction }, { previous: base.result }) };
      }
      return siblingOrderKept(base.graph, base.result, changed.result);
    };
    let single = 0;
    let incremental = 0;
    for (const id of ['network', 'security', 'application'] as const) {
      for (const change of ['collapse', 'refresh'] as const) {
        const [a, b] = [await keptBy(classic, id, change), await keptBy(inc, id, change)];
        single += a;
        incremental += b;
        expect(b, `${id} ${change}: incremental ${b.toFixed(2)} vs single-pass ${a.toFixed(2)}`).toBeGreaterThanOrEqual(0.6);
      }
    }
    expect(incremental, `total ${incremental.toFixed(2)} vs ${single.toFixed(2)}`).toBeGreaterThan(single);
  });

  it('routes cross-group edges as connected segments through boundary ports', async () => {
    const { graph, result } = await layoutOf('network', (s) => s, {}, inc);
    const crossing = graph.edges.find((e) => graph.nodes.get(e.source)!.parentId !== graph.nodes.get(e.target)!.parentId)!;
    const route = result.edges.get(crossing.id)!;
    expect(route.sections.length).toBeGreaterThan(1);
    for (let i = 1; i < route.sections.length; i++) {
      const end = route.sections[i - 1][route.sections[i - 1].length - 1];
      const start = route.sections[i][0];
      expect(Math.hypot(end.x - start.x, end.y - start.y), 'segments meet at a boundary port').toBeLessThan(12);
    }
  });
});

describe('layout signature', () => {
  it('is equal for equal graphs and changes with collapse', async () => {
    const p = projectionById('network');
    const view = project(index, p);
    const s = initialViewState(view);
    const a = layoutSignature(buildVisualGraph(view, index, s), p.layout);
    expect(layoutSignature(buildVisualGraph(view, index, s), p.layout)).toBe(a);
    expect(layoutSignature(buildVisualGraph(view, index, { ...s, collapsed: new Set(['g:aws:vpc-0prodcore']) }), p.layout)).not.toBe(a);
    expect(layoutSignature(buildVisualGraph(view, index, s), { ...p.layout, direction: 'DOWN' })).not.toBe(a);
  });
});
