import { describe, expect, it } from 'vitest';
import { buildDemoGraph, DEMO_VOCABULARY } from '../data/mock';
import { GraphIndex } from '../model/canonical';
import { project } from '../projection/project';
import { PROJECTIONS, projectionById } from '../projection/registry';
import type { SemanticView } from '../projection/types';
import { buildVisualGraph } from './build';
import type { VisualGraph } from './types';
import { initialViewState, revealed, type ViewState } from './viewState';

const index = new GraphIndex(buildDemoGraph());
const viewOf = (id: Parameters<typeof projectionById>[0]) => project(index, projectionById(id));
const PROD_CORE = 'g:aws:vpc-0prodcore';

/** Every drawn relationship's canonical endpoints map to the edge's visual endpoints. */
function assertEdgesFaithful(g: VisualGraph) {
  for (const e of g.edges) {
    for (const rid of e.relationshipIds) {
      const rel = index.relationship(rid)!;
      expect(index.vocab.relationshipType(rel.type).category).toBe(e.category);
      expect(g.resourceToNode.get(rel.source)).toBe(e.source);
      expect(g.resourceToNode.get(rel.target)).toBe(e.target);
    }
  }
}

function assertConserved(view: SemanticView, g: VisualGraph) {
  for (const rel of view.candidates) expect(g.dispositions.has(rel.id)).toBe(true);
  const drawn = new Set(g.edges.flatMap((e) => e.relationshipIds));
  for (const [rid, d] of g.dispositions) {
    if (d.kind === 'drawn') expect(drawn.has(rid)).toBe(true);
  }
  expect(drawn.size).toBe([...g.dispositions.values()].filter((d) => d.kind === 'drawn').length);
}

describe('visual graph', () => {
  it('is faithful and conserves relationships in every projection', () => {
    for (const p of PROJECTIONS) {
      const view = project(index, p);
      const state: ViewState = { ...initialViewState(view), enabledCategories: new Set(p.categories.available) };
      const g = buildVisualGraph(view, index, state);
      assertEdgesFaithful(g);
      assertConserved(view, g);
    }
  });

  it('nests visual nodes inside their groups, several levels deep', () => {
    const view = viewOf('network');
    const g = buildVisualGraph(view, index, initialViewState(view));
    const node = g.nodes.get('n:gcp:instance:acme-analytics/us-central1-a/etl-runner-1')!;
    const chain: string[] = [];
    for (let cur = node.parentId; cur; cur = g.nodes.get(cur)!.parentId) chain.push(g.nodes.get(cur)!.label);
    expect(chain).toEqual(['analytics-usc1', 'us-central1', 'shared-vpc', 'acme-net-host', 'production', 'example.com']);
    for (const n of g.nodes.values()) if (n.parentId) expect(g.nodes.get(n.parentId)!.children).toContain(n.id);
  });

  it('draws cross-hierarchy edges between nodes in different groups', () => {
    const view = viewOf('network');
    const g = buildVisualGraph(view, index, initialViewState(view));
    const owned = g.edges.find((e) => e.relationshipIds.includes('in_project:gcp:instance:acme-analytics/us-central1-a/etl-runner-1->gcp:project:acme-analytics'));
    expect(owned).toBeDefined();
    expect(owned!.target).toBe('g:gcp:project:acme-analytics');
    const flow = g.edges.find((e) => e.relationshipIds.includes('observed_flow:aws:i-0api1->aws:rds-orders'))!;
    expect(g.nodes.get(flow.source)!.parentId).not.toBe(g.nodes.get(flow.target)!.parentId);
  });

  it('attaches interface relationships to semantic ports', () => {
    const view = viewOf('network');
    const g = buildVisualGraph(view, index, initialViewState(view));
    const host = g.nodes.get('n:aws:i-0api1')!;
    const semantic = host.ports.filter((p) => p.kind === 'semantic').map((p) => p.resourceId);
    expect(semantic.sort()).toEqual(['aws:eni-0api1', 'aws:eni-0api1b']);
    const sgEdge = g.edges.find((e) => e.relationshipIds.includes('protected_by:aws:eni-0api1b->aws:sg-0api'))!;
    expect(sgEdge.sourcePort).toBe('p:aws:eni-0api1b');
  });

  it('expresses containment by nesting instead of edges', () => {
    const view = viewOf('network');
    const g = buildVisualGraph(view, index, initialViewState(view));
    expect(g.dispositions.get('in_subnet:aws:i-0api1->aws:subnet-0appa')?.kind).toBe('containment');
    // eth1 is in another subnet than its host: that one is drawn.
    expect(g.dispositions.get('in_subnet:aws:eni-0api1b->aws:subnet-0dataa')?.kind).toBe('drawn');
  });

  it('lifts edges to boundary ports when a group collapses, keeping membership', () => {
    const view = viewOf('network');
    const base = initialViewState(view);
    const g = buildVisualGraph(view, index, { ...base, collapsed: new Set([PROD_CORE]) });
    const node = g.nodes.get(PROD_CORE)!;
    expect(node.kind).toBe('collapsed');
    expect(node.memberCount).toBeGreaterThan(5);
    expect(g.nodes.has('n:aws:i-0api1')).toBe(false);
    expect(g.resourceToNode.get('aws:i-0api1')).toBe(PROD_CORE);
    // The cross-account SG rule now ends at the collapsed group's boundary.
    const lifted = g.edges.find((e) => e.relationshipIds.includes('allows_ingress_from:aws:sg-0api->aws:sg-0tools'))!;
    expect(lifted.liftedSource).toBe(PROD_CORE);
    expect(node.ports.find((p) => p.id === lifted.sourcePort)?.kind).toBe('boundary');
    // Relationships wholly inside are recorded, not dropped.
    expect(node.internalRelationships).toContain('observed_flow:aws:i-0api1->aws:rds-orders');
    assertEdgesFaithful(g);
    assertConserved(view, g);
  });

  it('aggregates parallel edges only within a category', () => {
    const view = viewOf('application');
    const base = initialViewState(view);
    const g = buildVisualGraph(view, index, { ...base, bundleParallel: true, enabledCategories: new Set(['dependency', 'traffic']) });
    const apiToDb = g.edges.filter((e) => e.source === 'n:aws:i-0api1' && e.target === 'n:aws:rds-orders');
    const dep = apiToDb.find((e) => e.category === 'dependency')!;
    expect(dep.aggregated).toBe(true);
    expect(dep.types.sort()).toEqual(['reads_from', 'writes_to']);
    // The observed flow stays its own edge: declared dependency is not observed traffic.
    expect(apiToDb.find((e) => e.category === 'traffic')?.relationshipIds).toEqual(['observed_flow:aws:i-0api1->aws:rds-orders']);
    // Unbundled, the same pair has one edge per relationship.
    const plain = buildVisualGraph(view, index, base);
    expect(plain.edges.filter((e) => e.source === 'n:aws:alb-orders' && e.target === 'n:aws:i-0api1')).toHaveLength(2);
  });

  it('never creates a transitive edge when an intermediate resource is hidden', () => {
    const view = viewOf('network');
    const base = initialViewState(view);
    const hidden = buildVisualGraph(view, index, { ...base, hiddenTypes: new Set(['aws.ec2.security_group']) });
    assertEdgesFaithful(hidden);
    expect(hidden.dispositions.get('allows_ingress_from:aws:sg-0db->aws:sg-0api')).toMatchObject({ kind: 'endpoint-hidden' });
    const before = new Set(buildVisualGraph(view, index, base).edges.flatMap((e) => e.relationshipIds));
    for (const e of hidden.edges) for (const rid of e.relationshipIds) expect(before.has(rid)).toBe(true);
  });

  it('removes out-of-scope owners without leaving their edges behind', () => {
    const view = viewOf('network');
    const base = initialViewState(view);
    const g = buildVisualGraph(view, index, { ...base, scope: new Set(['111111111111']) });
    expect([...g.nodes.values()].some((n) => n.label === 'shared-services')).toBe(false);
    expect(g.dispositions.get('allows_ingress_from:aws:sg-0api->aws:sg-0tools')).toMatchObject({ kind: 'endpoint-hidden' });
    assertEdgesFaithful(g);
  });

  it('focus keeps only the neighbourhood over enabled relationships', () => {
    const view = viewOf('application');
    const base = initialViewState(view);
    const g = buildVisualGraph(view, index, { ...base, focus: { resourceId: 'aws:sqs-orders', depth: 1 } });
    const shown = new Set([...g.nodes.values()].filter((n) => n.kind === 'resource').map((n) => n.resourceId));
    expect(shown).toEqual(new Set(['aws:sqs-orders', 'aws:i-0api1', 'aws:i-0api2', 'aws:i-0wrk1']));
    assertEdgesFaithful(g);
  });

  it('shows an unresolved placeholder only when one of its relationships is shown', () => {
    const view = viewOf('application');
    const base = initialViewState(view);
    expect(buildVisualGraph(view, index, base).nodes.has('n:aws:role:111111111111:bastion-legacy')).toBe(false);
    const withIam = buildVisualGraph(view, index, { ...base, enabledCategories: new Set([...base.enabledCategories, 'iam']) });
    expect(withIam.nodes.get('n:aws:role:111111111111:bastion-legacy')?.kind).toBe('unresolved');
  });

  it('flattens a group whose type is filtered, keeping its members', () => {
    const view = viewOf('network');
    const base = initialViewState(view);
    const g = buildVisualGraph(view, index, { ...base, hiddenTypes: new Set(['aws.ec2.subnet']) });
    expect(g.nodes.get('n:aws:i-0api1')?.parentId).toBe(PROD_CORE);
  });

  it('reveal expands every collapsed ancestor of a resource', () => {
    const view = viewOf('network');
    const collapsed = new Set([PROD_CORE, 'g:aws:account:111111111111', 'g:aws:vpc-0shared']);
    const next = revealed(view, collapsed, 'aws:i-0api1');
    expect([...next]).toEqual(['g:aws:vpc-0shared']);
  });
});

describe('render-only filters', () => {
  it('draw a subset of the laid-out graph, so filtering needs no relayout', async () => {
    const { layoutKey, layoutStateOf } = await import('./viewState');
    for (const p of PROJECTIONS) {
      const view = project(index, p);
      const base = initialViewState(view);
      const variants: ViewState[] = [
        { ...base, hiddenTypes: new Set(['aws.ec2.security_group', 'aws.ec2.subnet', 'gcp.iam.service_account']) },
        { ...base, scope: new Set(['111111111111', 'acme-analytics']) },
        { ...base, enabledCategories: new Set([p.categories.available[0]]) },
        { ...base, collapsed: new Set(['g:aws:vpc-0prodcore', 'g:aws:account:111111111111']), hiddenTypes: new Set(['aws.ec2.instance']) },
      ];
      for (const state of variants) {
        const layoutState = layoutStateOf(state, view);
        expect(layoutKey(layoutState)).toBe(layoutKey(layoutStateOf({ ...base, collapsed: state.collapsed }, view)));
        const laid = buildVisualGraph(view, index, layoutState);
        const shown = buildVisualGraph(view, index, state);
        for (const n of shown.nodes.values()) {
          const l = laid.nodes.get(n.id);
          expect(l, `${p.id} node ${n.id}`).toBeDefined();
          for (const port of n.ports) expect(l!.ports.some((q) => q.id === port.id), `${p.id} port ${port.id}`).toBe(true);
        }
        const laidEdges = new Set(laid.edges.map((e) => e.id));
        for (const e of shown.edges) expect(laidEdges.has(e.id), `${p.id} edge ${e.id}`).toBe(true);
      }
    }
  });
});

describe('synthetic owner groups', () => {
  it('use the owner type code when the owner was not collected', async () => {
    const { GraphBuilder } = await import('../data/mock/builder');
    const b = new GraphBuilder('gcp');
    b.res({ id: 'n1', type: 'gcp.compute.network', name: 'vpc', account: 'proj-x' });
    const idx = new GraphIndex({ meta: { origin: 'file', label: 't' }, resources: b.resources, relationships: [], vocabulary: DEMO_VOCABULARY });
    const view = project(idx, projectionById('network'));
    const g = buildVisualGraph(view, idx, initialViewState(view));
    const owner = [...g.nodes.values()].find((n) => n.label === 'proj-x')!;
    expect(owner.synthetic).toBe(true);
    expect(owner.code).toBe('PROJ');
  });
});
