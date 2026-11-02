import { describe, expect, it } from 'vitest';
import { GraphIndex, type CanonicalGraph } from '../model/canonical';
import { emptyPartial, mergeGraphs, notLoaded } from './merge';

const res = (id: string) => ({ id, type: 'aws.ec2.instance', name: id, provider: 'aws', properties: {} });
const rel = (s: string, t: string) => ({ id: `r:${s}->${t}`, type: 'in_subnet', source: s, target: t, basis: 'observed' as const });

describe('partial graphs', () => {
  it('merge neighbourhoods by ID and count unloaded relationships', () => {
    const meta = { origin: 'live' as const, label: 'x', partial: true };
    const a: CanonicalGraph = { meta: { ...meta, degree: { i1: 2, sub: 2 } }, resources: [res('i1'), res('sub')], relationships: [rel('i1', 'sub')] };
    let idx = new GraphIndex(a);
    expect(notLoaded(idx, 'i1')).toBe(1);
    expect(notLoaded(idx, 'sub')).toBe(1);

    const b: CanonicalGraph = { meta: { ...meta, degree: { i2: 1, sub: 2 } }, resources: [res('i2'), res('sub')], relationships: [rel('i2', 'sub')] };
    const merged = mergeGraphs(a, b);
    expect(merged.resources.map((r) => r.id).sort()).toEqual(['i1', 'i2', 'sub']);
    idx = new GraphIndex(merged);
    expect(notLoaded(idx, 'sub')).toBe(0);
    expect(notLoaded(idx, 'i1')).toBe(1);
    expect(merged.meta.partial).toBe(true);
  });

  it('report nothing unloaded for a full graph', () => {
    const full = new GraphIndex({ meta: { origin: 'live', label: 'x' }, resources: [res('i1')], relationships: [] });
    expect(notLoaded(full, 'i1')).toBe(0);
    expect(emptyPartial({ origin: 'live', label: 'x' }).meta).toMatchObject({ partial: true, degree: {} });
  });
});
