import { describe, expect, it } from 'vitest';
import { buildDemoGraph } from '../data/mock';
import { GraphIndex, type CanonicalGraph } from '../model/canonical';
import { project } from '../projection/project';
import { projectionById } from '../projection/registry';
import { initialViewState, isOwnerGroup, LARGE_VIEW } from './viewState';

const viewOf = (graph: CanonicalGraph) => project(new GraphIndex(graph), projectionById('network'));

describe('initialViewState', () => {
  it('leaves owners open in a small view', () => {
    const view = viewOf(buildDemoGraph());
    const owners = [...view.groups.values()].filter((g) => isOwnerGroup(g, view.vocab));
    expect(owners.length).toBeGreaterThan(0);
    for (const g of owners) expect(initialViewState(view).collapsed.has(g.id)).toBe(false);
  });

  it('collapses every owner in a large view', () => {
    const demo = buildDemoGraph();
    const host = demo.resources.find((r) => r.account && r.provider === 'aws')!;
    const extra = Array.from({ length: LARGE_VIEW }, (_, i) => ({ ...host, id: `${host.id}-copy-${i}`, name: `copy-${i}` }));
    const view = viewOf({ ...demo, resources: [...demo.resources, ...extra] });
    expect(view.members.size + view.groupOf.size).toBeGreaterThan(LARGE_VIEW);
    const state = initialViewState(view);
    const owners = [...view.groups.values()].filter((g) => isOwnerGroup(g, view.vocab));
    expect(owners.length).toBeGreaterThan(0);
    for (const g of owners) expect(state.collapsed.has(g.id)).toBe(true);
  });
});
