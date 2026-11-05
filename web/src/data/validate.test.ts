import { describe, expect, it } from 'vitest';
import { buildDemoGraph } from './mock';
import { parseCanonicalGraph, validateGraph } from './validate';

describe('graph validation', () => {
  it('reports dangling references in the demo graph without removing them', () => {
    const graph = buildDemoGraph();
    const issues = validateGraph(graph);
    expect(issues.map((i) => i.kind).sort()).toEqual(['dangling-target', 'dangling-target']);
    expect(graph.relationships.some((r) => r.target === 'aws:sg-0deadbeef')).toBe(true);
  });

  it('builds a deterministic demo graph that meets the dataset brief', () => {
    const a = buildDemoGraph();
    expect(JSON.stringify(a)).toBe(JSON.stringify(buildDemoGraph()));
    const types = (t: string) => a.resources.filter((r) => r.type === t);
    expect(types('aws.account').length + types('gcp.project').length).toBeGreaterThanOrEqual(2);
    expect(new Set(a.resources.map((r) => r.region).filter(Boolean)).size).toBeGreaterThanOrEqual(3);
    expect(types('aws.ec2.vpc').length + types('gcp.compute.network').length).toBeGreaterThanOrEqual(3);
    expect(a.meta.origin).toBe('mock');
  });

  it('flags duplicates and unknown relationship types', () => {
    const issues = validateGraph({
      meta: { origin: 'file', label: 't' },
      resources: [
        { id: 'a', type: 'x', name: 'a', provider: 'aws', properties: {} },
        { id: 'a', type: 'x', name: 'a', provider: 'aws', properties: {} },
      ],
      relationships: [{ id: 'r', type: 'frobnicates', source: 'a', target: 'a', basis: 'unknown' }],
      vocabulary: { categories: [], edgeTypes: [], providers: [], resourceTypes: [] },
    });
    expect(issues.map((i) => i.kind)).toEqual(['duplicate-resource', 'unknown-relationship-type']);
  });

  it('reports a graph without a vocabulary once, not per relationship', () => {
    const issues = validateGraph({
      meta: { origin: 'file', label: 't' },
      resources: [{ id: 'a', type: 'x', name: 'a', provider: 'aws', properties: {} }],
      relationships: [{ id: 'r', type: 'frobnicates', source: 'a', target: 'a', basis: 'unknown' }],
    });
    expect(issues.map((i) => i.kind)).toEqual(['no-vocabulary']);
  });

  it('never lets a file claim to be live inventory', () => {
    const g = parseCanonicalGraph({ meta: { origin: 'live', label: 'prod' }, resources: [], relationships: [] }, 'x.json');
    expect(g.meta.origin).toBe('file');
  });

  it('fills defaults: unknown basis, empty properties', () => {
    const g = parseCanonicalGraph(
      { resources: [{ id: 'a', type: 't' }], relationships: [{ id: 'r', type: 'reads_from', source: 'a', target: 'b' }] },
      'x.json',
    );
    expect(g.relationships[0].basis).toBe('unknown');
    expect(g.resources[0].properties).toEqual({});
  });

  it('rejects input that is not a graph', () => {
    expect(() => parseCanonicalGraph({ resources: 'nope' }, 'x')).toThrow(/resources array/);
  });
});

describe('pudl graph export documents', () => {
  it('keep producer warnings, schema and entry, and are still labelled as files', () => {
    const g = parseCanonicalGraph(
      {
        meta: { origin: 'pudl', label: 'pudl catalog · demo', warnings: ['ref ambiguous', 3] },
        resources: [{ id: 'r1', type: 'aws.ec2.instance', name: 'i-1', provider: 'aws', schema: 'aws.#EC2Instance', entryId: 'babod-fakak' }],
        relationships: [{ id: 'x', type: 'protected_by', source: 'r1', target: 'ref:sg-1', basis: 'observed' }],
        vocabulary: {
          categories: [{ id: 'security', label: 'Security control', meaning: 'm', order: 4, source: 'graph_edge_category_security @ x.cue:1' }],
          edgeTypes: [{ type: 'protected_by', category: 'security', label: 'SG', meaning: 'm' }, { type: 3 }],
          providers: [{ provider: 'aws', label: 'AWS' }],
          resourceTypes: [{ type: 'aws.ec2.instance', code: 'EC2', label: 'EC2 instance', class: 'compute', schema: 'aws.#EC2Instance', admin: null, network: { via: ['in_subnet'] }, portOf: '', security: null, inventory: null, application: null }],
          problems: ['p'],
        },
      },
      'graph.json',
    );
    expect(g.meta).toMatchObject({ origin: 'file', label: 'pudl catalog · demo', warnings: ['ref ambiguous'] });
    expect(g.resources[0]).toMatchObject({ schema: 'aws.#EC2Instance', entryId: 'babod-fakak' });
    expect(validateGraph(g).map((i) => i.kind)).toEqual(['dangling-target']);
    expect(g.vocabulary?.edgeTypes.map((e) => e.type)).toEqual(['protected_by']);
    expect(g.vocabulary?.resourceTypes[0].network).toEqual({ via: ['in_subnet'] });
    expect(g.vocabulary?.problems).toEqual(['p']);
  });
});

describe('origin', () => {
  it('comes from the source, never from the document', () => {
    const doc = { meta: { origin: 'mock', label: 'x' }, resources: [], relationships: [] };
    expect(parseCanonicalGraph(doc, 'x').meta.origin).toBe('file');
    expect(parseCanonicalGraph(doc, 'x', 'live').meta.origin).toBe('live');
  });

  it('keeps the records of a resource that several schemas describe', () => {
    const graph = parseCanonicalGraph(
      {
        resources: [
          {
            id: 'r-gke',
            type: 'gcp.container.cluster',
            representations: [
              { id: 'r-gke', schema: 'gcp.#GKECluster', type: 'gcp.container.cluster', entryId: 'e1', origin: 'gcp' },
              { id: 'r-ctx', schema: 'k8s.#Cluster', type: 'k8s.cluster', entryId: 'e2', origin: 'kubectl' },
            ],
          },
        ],
        relationships: [],
      },
      'test',
    );
    expect(graph.resources[0].representations?.map((r) => r.schema)).toEqual(['gcp.#GKECluster', 'k8s.#Cluster']);
  });
});
