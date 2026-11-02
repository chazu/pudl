import { describe, expect, it } from 'vitest';
import { buildDemoGraph } from '../data/mock';
import { GraphIndex } from '../model/canonical';
import { project } from '../projection/project';
import { PROJECTIONS, projectionById } from '../projection/registry';
import type { ProjectionId } from '../projection/types';
import { buildVisualGraph } from '../visual/build';
import type { VisualGraph } from '../visual/types';
import { initialViewState, type ViewState } from '../visual/viewState';
import { ElkLayoutEngine } from './elkAdapter';
import type { Box, LayoutResult } from './types';

const index = new GraphIndex(buildDemoGraph());
const engine = new ElkLayoutEngine();

async function layoutOf(id: ProjectionId, tweak: (s: ViewState) => ViewState = (s) => s) {
  const p = projectionById(id);
  const view = project(index, p);
  const state = tweak(initialViewState(view));
  const graph = buildVisualGraph(view, index, state);
  const result = await engine.layout(graph, { ...p.layout, direction: state.direction }, {});
  return { graph, result };
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

describe('ELK adapter', () => {
  for (const p of PROJECTIONS) {
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
