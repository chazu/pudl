import { describe, expect, it } from 'vitest';
import { rankSearch, searchRank } from './searchRank';

const res = (id: string, name: string, nativeId = `projects/p/${name}`) => ({ id, name, type: 't.x', nativeId });

describe('rankSearch', () => {
  it('puts the exact name first, then prefixes, then shorter substrings', () => {
    const items = [res('a', 'gke-core-a1f9-subnet'), res('b', 'gke-core-a1f9'), res('c', 'core'), res('d', 'my-core'), res('e', 'core-ingress')];
    expect(rankSearch(items, ' Core ', 10).map((r) => r.name)).toEqual(['core', 'core-ingress', 'my-core', 'gke-core-a1f9', 'gke-core-a1f9-subnet']);
  });

  it('matches native IDs, types and IDs below names', () => {
    expect(searchRank(res('x', 'web', 'projects/p/zones/z/instances/i-123'), 'i-123')).toBe(3);
    expect(searchRank(res('x', 'web', 'arn:aws:i-123456'), 'i-123')).toBe(4);
    expect(searchRank(res('x', 'web'), 't.x')).toBe(5);
    expect(searchRank(res('x', 'web'), 'nothing')).toBe(-1);
  });

  it('applies the limit and ignores a blank query', () => {
    expect(rankSearch([res('a', 'aa'), res('b', 'ab'), res('c', 'ac')], 'a', 2)).toHaveLength(2);
    expect(rankSearch([res('a', 'aa')], '  ', 10)).toEqual([]);
  });
});
