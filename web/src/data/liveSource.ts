import type { CanonicalGraph } from '../model/canonical';
import type { GraphSource } from './source';
import { parseCanonicalGraph } from './validate';

/**
 * The workspace catalog, read live from the `pudl ui` server that served this
 * page (same origin, /api/graph). Each load is the catalog's current state.
 */
export class LiveGraphSource implements GraphSource {
  readonly id = 'live';
  readonly label = 'pudl ui';

  async load(): Promise<CanonicalGraph> {
    const res = await fetch('/api/graph', { cache: 'no-store' });
    const body = await res.json().catch(() => null);
    if (!res.ok) throw new Error(`pudl ui: ${body?.error ?? res.statusText}`);
    return parseCanonicalGraph(body, 'pudl catalog', 'live');
  }
}

/** True when this page was served by `pudl ui` (its /api/health identifies pudl). */
export async function servedByPudl(timeoutMs = 1500): Promise<boolean> {
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), timeoutMs);
  try {
    const res = await fetch('/api/health', { signal: ctrl.signal, cache: 'no-store' });
    if (!res.ok) return false;
    const body = await res.json();
    return body?.pudl === true;
  } catch {
    return false;
  } finally {
    clearTimeout(timer);
  }
}
