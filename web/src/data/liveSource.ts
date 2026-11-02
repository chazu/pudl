import type { CanonicalGraph } from '../model/canonical';
import type { GraphSource, GraphSummary, LoadOptions, SearchHit, SnapshotInfo } from './source';

function query(params: Record<string, string | undefined>): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v) q.set(k, v);
  const s = q.toString();
  return s ? `?${s}` : '';
}

async function getJSON(url: string): Promise<unknown> {
  const res = await fetch(url, { cache: 'no-store' });
  const body = await res.json().catch(() => null);
  if (!res.ok) throw new Error(`pudl ui: ${(body as { error?: string } | null)?.error ?? res.statusText}`);
  return body;
}
import { parseCanonicalGraph } from './validate';

/**
 * The workspace catalog, read live from the `pudl ui` server that served this
 * page (same origin, /api/graph). Each load is the catalog's current state.
 */
export class LiveGraphSource implements GraphSource {
  readonly id = 'live';
  readonly label = 'pudl ui';

  async load(opts: LoadOptions = {}): Promise<CanonicalGraph> {
    return parseCanonicalGraph(await getJSON(`/api/graph${query({ as_of: opts.asOf })}`), 'pudl catalog', 'live');
  }

  async summary(opts: LoadOptions = {}): Promise<GraphSummary> {
    return (await getJSON(`/api/summary${query({ as_of: opts.asOf })}`)) as GraphSummary;
  }

  async neighbourhood(ids: string[], closure: string[], opts: LoadOptions = {}): Promise<CanonicalGraph> {
    const url = `/api/graph${query({ focus: ids.join(','), depth: '1', closure: closure.join(','), as_of: opts.asOf })}`;
    return parseCanonicalGraph(await getJSON(url), 'pudl catalog', 'live');
  }

  async search(q: string, opts: LoadOptions = {}): Promise<SearchHit[]> {
    const body = await getJSON(`/api/search${query({ q, limit: '40', as_of: opts.asOf })}`);
    return Array.isArray(body) ? body : [];
  }

  async snapshots(): Promise<SnapshotInfo[]> {
    const res = await fetch('/api/snapshots', { cache: 'no-store' });
    if (!res.ok) return [];
    const body = await res.json();
    return Array.isArray(body) ? body : [];
  }

  subscribe(onVersion: (version: string) => void, onFail: () => void): () => void {
    if (typeof EventSource === 'undefined') {
      onFail();
      return () => {};
    }
    const es = new EventSource('/api/events');
    es.addEventListener('version', (e) => onVersion((e as MessageEvent<string>).data));
    // EventSource retries transient errors itself; CLOSED means it gave up
    // (for example an older pudl ui without /api/events).
    es.onerror = () => {
      if (es.readyState === EventSource.CLOSED) onFail();
    };
    return () => es.close();
  }

  async version(): Promise<string | null> {
    try {
      const res = await fetch('/api/version', { cache: 'no-store' });
      if (!res.ok) return null;
      const body = await res.json();
      return typeof body?.version === 'string' ? body.version : null;
    } catch {
      return null;
    }
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
