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
    });
    expect(issues.map((i) => i.kind)).toEqual(['duplicate-resource', 'unknown-relationship-type']);
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
