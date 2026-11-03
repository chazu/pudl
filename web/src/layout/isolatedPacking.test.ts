import { describe, expect, it } from 'vitest';
import { DEMO_VOCABULARY } from '../data/mock';
import { GraphBuilder } from '../data/mock/builder';
import { GraphIndex } from '../model/canonical';
import { project } from '../projection/project';
import { projectionById } from '../projection/registry';
import { buildVisualGraph } from '../visual/build';
import { initialViewState } from '../visual/viewState';
import { ElkLayoutEngine } from './elkAdapter';
import { isolatedPlan } from './isolatedPacking';

describe('unconnected leaves', () => {
  it('are packed into a grid inside their group, without overlaps', async () => {
    const b = new GraphBuilder('aws');
    b.res({ id: 'acct', type: 'aws.account', name: 'acct', nativeId: '1', account: '1' });
    for (let i = 0; i < 12; i++) b.res({ id: `u${i}`, type: 'aws.iam.user', name: `user-${i}`, account: '1' });
    b.res({ id: 'r', type: 'aws.iam.role', name: 'role', account: '1' });
    b.res({ id: 'p', type: 'aws.iam.policy', name: 'policy', account: '1' });
    b.rel('has_policy', 'r', 'p', 'observed');
    const idx = new GraphIndex({ meta: { origin: 'file', label: 't' }, resources: b.resources, relationships: b.relationships, vocabulary: DEMO_VOCABULARY });
    const view = project(idx, projectionById('security'));
    const state = initialViewState(view);
    const graph = buildVisualGraph(view, idx, state);

    const plan = isolatedPlan(graph);
    expect([...plan.values()].map((p) => p.members.length)).toEqual([12]);
    expect([...plan.values()][0].cols).toBeGreaterThan(1);

    const result = await new ElkLayoutEngine().layout(graph, { algorithm: 'layered', direction: state.direction }, {});
    const group = result.nodes.get('g:acct')!;
    const boxes = [...Array(12).keys()].map((i) => result.nodes.get(`n:u${i}`)!);
    for (const box of boxes) {
      expect(box.x).toBeGreaterThanOrEqual(group.x);
      expect(box.x + box.width).toBeLessThanOrEqual(group.x + group.width + 0.5);
      expect(box.y + box.height).toBeLessThanOrEqual(group.y + group.height + 0.5);
    }
    for (let i = 0; i < boxes.length; i++) {
      for (let j = i + 1; j < boxes.length; j++) {
        const [a, c] = [boxes[i], boxes[j]];
        const overlap = a.x < c.x + c.width && c.x < a.x + a.width && a.y < c.y + c.height && c.y < a.y + a.height;
        expect(overlap).toBe(false);
      }
    }
    expect(group.width / group.height).toBeLessThan(4);
    // Connected nodes still go through ELK and get routed edges.
    expect(result.edges.get(graph.edges[0].id)?.routed).toBe(true);
  });
});
