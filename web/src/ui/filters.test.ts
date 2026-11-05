import { describe, expect, it } from 'vitest';
import { isOnlyOwner, isOnlyType, onlyOwner, onlyType } from './filters';

describe('only: scope', () => {
  it('isolates an owner, and a second click shows every owner', () => {
    const isolated = onlyOwner(null, 'proj-a');
    expect([...isolated!]).toEqual(['proj-a']);
    expect(isOnlyOwner(isolated, 'proj-a')).toBe(true);
    expect(onlyOwner(isolated, 'proj-a')).toBeNull();
  });

  it('switches isolation to another owner', () => {
    expect([...onlyOwner(new Set(['proj-a']), 'proj-b')!]).toEqual(['proj-b']);
    expect(isOnlyOwner(new Set(['proj-a', 'proj-b']), 'proj-a')).toBe(false);
    expect(isOnlyOwner(null, 'proj-a')).toBe(false);
  });
});

describe('only: resource types', () => {
  const all = ['gcp.container.cluster', 'gcp.k8s.deployment', 'gcp.compute.subnetwork'];

  it('hides every other type, and a second click shows every type', () => {
    const hidden = onlyType(new Set(), 'gcp.k8s.deployment', all);
    expect([...hidden].sort()).toEqual(['gcp.compute.subnetwork', 'gcp.container.cluster']);
    expect(isOnlyType(hidden, 'gcp.k8s.deployment', all)).toBe(true);
    expect(onlyType(hidden, 'gcp.k8s.deployment', all).size).toBe(0);
  });

  it('isolates a type that was hidden', () => {
    const hidden = onlyType(new Set(['gcp.k8s.deployment']), 'gcp.k8s.deployment', all);
    expect(hidden.has('gcp.k8s.deployment')).toBe(false);
    expect(hidden.size).toBe(2);
  });

  it('is not "only" while another type is still shown', () => {
    expect(isOnlyType(new Set(['gcp.compute.subnetwork']), 'gcp.k8s.deployment', all)).toBe(false);
    expect(isOnlyType(new Set(['gcp.compute.subnetwork', 'gcp.container.cluster', 'gcp.k8s.deployment']), 'gcp.k8s.deployment', all)).toBe(false);
  });
});
