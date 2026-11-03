import { describe, expect, it } from 'vitest';
import { buildDemoGraph } from '../data/mock';
import { GraphBuilder } from '../data/mock/builder';
import { GraphIndex, type CanonicalGraph } from '../model/canonical';
import { groupAncestors, project } from './project';
import { PROJECTIONS, projectionById } from './registry';
import { UNRESOLVED_GROUP_ID, type ProjectionDefinition, type SemanticView } from './types';

const demo = new GraphIndex(buildDemoGraph());
const pathOf = (view: SemanticView, id: string) => {
  const m = view.members.get(id);
  const gid = view.groupOf.get(id);
  const start = m?.parentId ?? (gid ? view.groups.get(gid)?.parentId : null) ?? null;
  return groupAncestors(view.groups, start)
    .reverse()
    .map((g) => view.groups.get(g)!.label);
};

const ETL1 = 'gcp:instance:acme-analytics/us-central1-a/etl-runner-1';

describe('projections over the demo graph', () => {
  const views = new Map(PROJECTIONS.map((p) => [p.id, project(demo, p)]));

  it('all reference canonical identities only', () => {
    for (const view of views.values()) {
      for (const m of view.members.values()) {
        if (m.role === 'unresolved') expect(demo.has(m.resourceId)).toBe(false);
        else expect(demo.has(m.resourceId)).toBe(true);
      }
      for (const g of view.groups.values()) {
        if (g.resourceId) expect(demo.has(g.resourceId)).toBe(true);
      }
    }
  });

  it('give every canonical relationship a disposition', () => {
    for (const view of views.values()) {
      for (const rel of demo.graph.relationships) expect(view.dispositions.has(rel.id)).toBe(true);
    }
  });

  it('do not mutate the canonical graph', () => {
    const fresh = buildDemoGraph();
    expect(JSON.stringify(demo.graph)).toBe(JSON.stringify(fresh));
  });

  it('place a Shared VPC instance by network in one view and by owner in another', () => {
    const net = views.get('network')!;
    expect(pathOf(net, ETL1)).toEqual([
      'example.com',
      'production',
      'acme-net-host',
      'shared-vpc',
      'us-central1',
      'analytics-usc1',
    ]);
    const inv = views.get('inventory')!;
    expect(pathOf(inv, ETL1)).toEqual(['Google Cloud', 'example.com', 'production', 'acme-analytics', 'us-central1', 'Compute instance']);
    const app = views.get('application')!;
    expect(pathOf(app, ETL1)).toEqual(['Analytics', 'etl']);
  });

  it('uses provider-aware network hierarchies (region outside the VPC on AWS, inside the network on GCP)', () => {
    const net = views.get('network')!;
    expect(pathOf(net, 'aws:i-0api1')).toEqual(['prod-app', 'us-east-1', 'prod-core', 'app-a']);
    expect(net.groups.get('g:aws:vpc-0prodcore')?.parentId).toMatch(/region:us-east-1$/);
    expect(net.groups.get(net.groupOf.get('gcp:subnet:acme-net-host/us-central1/analytics-usc1')!)?.parentId).toMatch(
      /shared-vpc\/region:us-central1$/,
    );
  });

  it('places multi-parent resources at the common ancestor and says so', () => {
    const net = views.get('network')!;
    const alb = net.members.get('aws:alb-orders')!;
    expect(alb.parentId).toBe('g:aws:vpc-0prodcore');
    expect(alb.placementNote).toMatch(/2 placement parents/);
    const pcx = net.members.get('aws:pcx-0shared')!;
    expect(pcx.parentId).toBeNull();
    expect(pcx.placementNote).toMatch(/top level/);
  });

  it('renders interfaces as ports on their hosts', () => {
    const net = views.get('network')!;
    const eth1 = net.members.get('aws:eni-0api1b')!;
    expect(eth1.role).toBe('port');
    expect(eth1.hostId).toBe('aws:i-0api1');
    expect(net.dispositions.get('attached_to:aws:eni-0api1b->aws:i-0api1')).toEqual({ kind: 'port-binding' });
  });

  it('keeps references to uncollected resources as unresolved placeholders', () => {
    const net = views.get('network')!;
    const ph = net.members.get('aws:sg-0deadbeef')!;
    expect(ph.role).toBe('unresolved');
    expect(ph.parentId).toBe(UNRESOLVED_GROUP_ID);
    expect(net.dispositions.get('allows_ingress_from:aws:sg-0api->aws:sg-0deadbeef')).toEqual({ kind: 'candidate' });
  });

  it('keeps security relationships out of the application view by default and network out of security', () => {
    expect(projectionById('application').categories.enabled).not.toContain('security');
    expect(projectionById('security').categories.enabled).not.toContain('network');
    expect(projectionById('network').categories.enabled).not.toContain('iam');
  });

  it('puts dependency participants without membership in an explicit unassigned group', () => {
    const app = views.get('application')!;
    // The orders worker writes to the analytics bucket; that bucket is a member, so check a pure participant:
    for (const m of app.members.values()) {
      if (m.role !== 'node') continue;
      const r = demo.resource(m.resourceId)!;
      if (demo.out(r.id, 'member_of').length === 0) expect(m.parentId).toBe('s:app:unassigned');
    }
  });
});

function fixture(build: (b: GraphBuilder) => void): GraphIndex {
  const b = new GraphBuilder('aws');
  build(b);
  const graph: CanonicalGraph = { meta: { origin: 'mock', label: 'fixture' }, resources: b.resources, relationships: b.relationships };
  return new GraphIndex(graph);
}

const boxes: ProjectionDefinition = {
  ...projectionById('network'),
  include: () => true,
  isGroup: (r) => r.type === 'box',
  place: (r, ctx) => ({ parents: ctx.via(r, 'in'), basis: 'test' }),
  ports: () => [],
  categories: { available: ['dependency', 'other'], enabled: ['dependency', 'other'] },
};

describe('projection edge cases', () => {
  it('survives a placement cycle between groups', () => {
    const idx = fixture((b) => {
      b.res({ id: 'A', type: 'box', name: 'A' });
      b.res({ id: 'B', type: 'box', name: 'B' });
      b.rel('in', 'A', 'B', 'observed');
      b.rel('in', 'B', 'A', 'observed');
    });
    const view = project(idx, boxes);
    expect(view.groups.size).toBe(2);
    for (const g of view.groups.values()) expect(groupAncestors(view.groups, g.id).length).toBeLessThanOrEqual(2);
  });

  it('places a resource whose parent is missing at top level with a note', () => {
    const idx = fixture((b) => {
      b.res({ id: 'x', type: 'leaf', name: 'x' });
      b.rel('in', 'x', 'ghost', 'observed');
    });
    const view = project(idx, { ...boxes, place: () => ({ parents: [{ resource: 'ghost' }], basis: 't' }) });
    expect(view.members.get('x')?.parentId).toBeNull();
    expect(view.members.get('x')?.placementNote).toMatch(/not in this view/);
  });

  it('does not invent a placeholder when both ends are missing', () => {
    const idx = fixture((b) => {
      b.rel('reads_from', 'ghost1', 'ghost2', 'declared');
    });
    const view = project(idx, boxes);
    expect(view.members.size).toBe(0);
    expect(view.candidates).toHaveLength(0);
  });

  it('treats unregistered relationship types as "other", never as a stronger category', () => {
    const idx = fixture((b) => {
      b.res({ id: 'a', type: 'leaf', name: 'a' });
      b.res({ id: 'b', type: 'leaf', name: 'b' });
      b.rel('mystery_link', 'a', 'b', 'unknown');
    });
    const view = project(idx, { ...boxes, categories: { available: ['dependency'], enabled: ['dependency'] } });
    expect(view.dispositions.get('mystery_link:a->b')).toEqual({ kind: 'category-not-in-view', category: 'other' });
  });
});
