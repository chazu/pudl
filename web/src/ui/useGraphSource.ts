// Where the graph comes from and when it is (re)loaded: source selection,
// reload, as-of time, auto-refresh, and partial loading for large
// inventories.
//
// Partial mode: when a live source reports more resources than the
// threshold (?partialAbove=N, default PARTIAL_ABOVE; ?full disables it), the
// UI starts empty and loads neighbourhoods on demand (search, "Load
// neighbours"). Loaded neighbourhoods are merged; a reload re-fetches every
// neighbourhood loaded so far.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { emptyPartial, mergeGraphs } from '../data/merge';
import { loadFrom, sourceFromLocation, type GraphSource, type LoadedGraph, type LoadOptions, type SearchHit } from '../data/source';
import { validateGraph } from '../data/validate';
import { Vocab } from '../model/vocabulary';
import { useAutoRefresh } from './useAutoRefresh';

export const PARTIAL_ABOVE = 500;

const withIssues = (graph: LoadedGraph['graph']): LoadedGraph => ({ graph, issues: validateGraph(graph) });

export function useGraphSource() {
  const params = useMemo(() => new URLSearchParams(window.location.search), []);
  const [source, setSource] = useState<GraphSource | null>(null);
  const [loaded, setLoaded] = useState<LoadedGraph | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [reloading, setReloading] = useState(false);
  const [reloadError, setReloadError] = useState<string | null>(null);
  const [asOf, setAsOf] = useState<string | null>(null);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [partial, setPartial] = useState(false);
  const focus = useRef<string[]>([]);
  // Containment types from the last vocabulary seen: followed when loading a
  // neighbourhood. Empty before the first neighbourhood arrives; the server
  // then follows containment itself.
  const containment = useRef<string[]>([]);
  useEffect(() => {
    if (loaded?.graph.vocabulary) containment.current = new Vocab(loaded.graph.vocabulary).containmentTypes();
  }, [loaded]);

  const fetchGraph = useCallback(async (s: GraphSource, at: string | null, isPartial: boolean): Promise<LoadedGraph> => {
    const opts: LoadOptions | undefined = at ? { asOf: at } : undefined;
    if (!isPartial) return loadFrom(s, opts);
    if (focus.current.length === 0) {
      const summary = await s.summary!(opts);
      const version = (summary as { version?: string }).version;
      return withIssues(
        emptyPartial({ origin: 'live', label: 'pudl catalog', asOf: at ?? undefined, version, totals: summary }),
      );
    }
    return withIssues(await s.neighbourhood!(focus.current, containment.current, opts));
  }, []);

  useEffect(() => {
    (async () => {
      const s = await sourceFromLocation(window.location.search);
      setSource(s);
      let isPartial = false;
      if (s.summary && s.neighbourhood && !params.has('full')) {
        const threshold = Number(params.get('partialAbove') ?? PARTIAL_ABOVE);
        isPartial = (await s.summary()).resources > threshold;
      }
      setPartial(isPartial);
      setLoaded(await fetchGraph(s, null, isPartial));
    })().catch((e: unknown) => setLoadError(String(e)));
  }, [params, fetchGraph]);

  // Reloading keeps view state and selection: resource and group IDs are stable.
  const reloadAt = useCallback(
    (at: string | null) => {
      if (!source) return;
      setReloading(true);
      fetchGraph(source, at, partial)
        .then((g) => {
          setLoaded(g);
          setReloadError(null);
        })
        .catch((e: unknown) => setReloadError(String(e)))
        .finally(() => setReloading(false));
    },
    [source, partial, fetchGraph],
  );
  const reload = useCallback(() => reloadAt(asOf), [reloadAt, asOf]);
  const changeAsOf = useCallback(
    (at: string | null) => {
      setAsOf(at);
      reloadAt(at);
    },
    [reloadAt],
  );

  // The past does not change: no polling while viewing a past time.
  useAutoRefresh(source, loaded?.graph.meta.version, autoRefresh && !asOf, reloading, reload);

  /** Load the neighbourhoods of `ids` and merge them in (partial mode only). */
  const expand = useCallback(
    async (ids: string[]) => {
      if (!partial || !source?.neighbourhood) return;
      const fresh = ids.filter((id) => !focus.current.includes(id));
      if (fresh.length === 0) return;
      focus.current = [...focus.current, ...fresh];
      setReloading(true);
      try {
        const add = await source.neighbourhood(fresh, containment.current, asOf ? { asOf } : undefined);
        setLoaded((cur) => withIssues(cur && cur.graph.resources.length > 0 ? mergeGraphs(cur.graph, add) : add));
        setReloadError(null);
      } catch (e) {
        setReloadError(String(e));
      } finally {
        setReloading(false);
      }
    },
    [partial, source, asOf],
  );

  const search = useMemo(
    () => (partial && source?.search ? (q: string): Promise<SearchHit[]> => source.search!(q, asOf ? { asOf } : undefined) : undefined),
    [partial, source, asOf],
  );

  return {
    source,
    loaded,
    loadError,
    reloading,
    reloadError,
    reload,
    asOf,
    setAsOf: changeAsOf,
    autoRefresh,
    setAutoRefresh,
    partial,
    expand,
    search,
  };
}
