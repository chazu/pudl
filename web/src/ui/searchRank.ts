// Search ranking for the inventory search box. Mirrors the pudl ui server's
// ranking (internal/graphexport/subgraph.go searchRank), so local and remote
// search order results the same way.

export interface Searchable {
  id: string;
  name: string;
  type: string;
  nativeId?: string;
}

/**
 * Rank of one resource for a lower-cased, trimmed query: exact name, name
 * prefix, name substring, exact native ID (or its last path segment),
 * native ID substring, then type or ID. -1: no match.
 */
export function searchRank(r: Searchable, q: string): number {
  const name = r.name.toLowerCase();
  const native = (r.nativeId ?? '').toLowerCase();
  if (name === q) return 0;
  if (name.startsWith(q)) return 1;
  if (name.includes(q)) return 2;
  if (native === q || native.endsWith(`/${q}`)) return 3;
  if (native.includes(q)) return 4;
  if (r.type.toLowerCase().includes(q) || r.id.toLowerCase().includes(q)) return 5;
  return -1;
}

/** The best `limit` matches for `query`, best first; shorter names win ties. */
export function rankSearch<T extends Searchable>(items: readonly T[], query: string, limit: number): T[] {
  const q = query.trim().toLowerCase();
  if (!q) return [];
  const scored: { item: T; rank: number }[] = [];
  for (const item of items) {
    const rank = searchRank(item, q);
    if (rank >= 0) scored.push({ item, rank });
  }
  scored.sort((a, b) => a.rank - b.rank || a.item.name.length - b.item.name.length || (a.item.name < b.item.name ? -1 : a.item.name > b.item.name ? 1 : 0));
  return scored.slice(0, limit).map((s) => s.item);
}
