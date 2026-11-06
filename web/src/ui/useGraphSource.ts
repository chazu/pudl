// Where the graph comes from and when it is (re)loaded: source selection,
// reload, as-of time, auto-refresh, and partial loading for large
// inventories.
//
// Partial mode: when a live source reports more resources than the
// threshold (?partialAbove=N, default PARTIAL_ABOVE; ?full disables it), the
// UI starts on the administrative overview (organizations, folders,
// accounts, projects) and loads more on demand: a resource's neighbourhood
// (search, "Load neighbours") or what an owner owns (its ⋯ badge). Loaded
// parts are merged; a reload re-fetches every part loaded so far.

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
  const owners = useRef<string[]>([]);
  // Has anything been loaded beyond the overview?
  const [expanded, setExpanded] = useState(false);
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
    let graph;
    if (s.overview) graph = await s.overview(containment.current, opts);
    else {
      const summary = await s.summary!(opts);
      const version = (summary as { version?: string }).version;
      graph = emptyPartial({ origin: 'live', label: 'pudl catalog', asOf: at ?? undefined, version, totals: summary });
    }
    if (focus.current.length > 0) graph = mergeGraphs(graph, await s.neighbourhood!(focus.current, containment.current, opts));
    if (owners.current.length > 0 && s.owned) graph = mergeGraphs(graph, await s.owned(owners.current, containment.current, opts));
    return withIssues(graph);
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

  /**
   * Load the neighbourhoods of `ids`, and for owners among them what they
   * own, and merge them in (partial mode only).
   */
  const expand = useCallback(
    async (ids: string[]) => {
      if (!partial || !source?.neighbourhood) return;
      const fresh = ids.filter((id) => !focus.current.includes(id));
      const owned = loaded?.graph.meta.owned ?? {};
      const freshOwners = source.owned ? ids.filter((id) => (owned[id] ?? 0) > 0 && !owners.current.includes(id)) : [];
      if (fresh.length === 0 && freshOwners.length === 0) return;
      focus.current = [...focus.current, ...fresh];
      owners.current = [...owners.current, ...freshOwners];
      setExpanded(true);
      setReloading(true);
      try {
        const opts = asOf ? { asOf } : undefined;
        const parts = await Promise.all([
          fresh.length ? source.neighbourhood(fresh, containment.current, opts) : null,
          freshOwners.length ? source.owned!(freshOwners, containment.current, opts) : null,
        ]);
        setLoaded((cur) => {
          let graph = cur && cur.graph.resources.length > 0 ? cur.graph : null;
          for (const add of parts) if (add) graph = graph ? mergeGraphs(graph, add) : add;
          return graph ? withIssues(graph) : cur;
        });
        setReloadError(null);
      } catch (e) {
        setReloadError(String(e));
      } finally {
        setReloading(false);
      }
    },
    [partial, source, asOf, loaded],
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
    expanded,
    expand,
    search,
  };
}
