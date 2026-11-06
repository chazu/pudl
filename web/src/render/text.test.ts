import { describe, expect, it } from 'vitest';
import { middleEllipsis } from './text';

describe('middleEllipsis', () => {
  it('keeps short strings', () => expect(middleEllipsis('subnet', 10)).toBe('subnet'));

  it('cuts from the middle, keeping the distinguishing end', () => {
    const a = middleEllipsis('gke-example-cluster-a1f9e2-pe-subnet', 20);
    const b = middleEllipsis('gke-example-cluster-a1f9e2-ingress', 20);
    expect(a).toHaveLength(20);
    expect(a.startsWith('gke-grou')).toBe(true);
    expect(a.endsWith('pe-subnet')).toBe(true);
    expect(a).not.toBe(b);
  });

  it('handles tiny widths', () => expect(middleEllipsis('abcdef', 1)).toBe('…'));
});
