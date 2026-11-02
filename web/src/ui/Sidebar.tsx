import { useEffect, useMemo, useState, type ReactNode } from 'react';
import type { SearchHit } from '../data/source';
import type { GraphIndex, GraphMeta, ResourceId } from '../model/canonical';
import { resourceType } from '../model/resourceTypes';
import { PROJECTIONS } from '../projection/registry';
import { ownerResource } from '../projection/views/common';
import type { ProjectionId, SemanticView } from '../projection/types';
import type { Selection } from '../render/highlight';
import type { ViewState } from '../visual/viewState';
import { HierarchyTree } from './HierarchyTree';

interface Props {
  meta: GraphMeta;
  /** Re-read the source; absent for the demo dataset. */
  onReload?: () => void;
  reloading?: boolean;
  reloadError?: string | null;
  /** Auto-refresh toggle state; absent when the source cannot report changes. */
  autoRefresh?: boolean;
  onAutoRefresh?: (on: boolean) => void;
  /** As-of picker, for sources with history. */
  timeControl?: ReactNode;
  /** Partial graphs: search the whole inventory on the server; picking a hit loads it. */
  remoteSearch?: (q: string) => Promise<SearchHit[]>;
  onPickRemote?: (id: ResourceId) => void;
  index: GraphIndex;
  views: Map<ProjectionId, SemanticView>;
  projectionId: ProjectionId;
  state: ViewState;
  selection: Selection;
  onProjection(id: ProjectionId): void;
  onSelectResource(id: ResourceId): void;
  onSelectGroup(id: string): void;
  onToggleGroup(id: string): void;
  onState(fn: (s: ViewState) => ViewState): void;
}

interface BannerProps {
  meta: GraphMeta;
  onReload?: () => void;
  reloading?: boolean;
  reloadError?: string | null;
  autoRefresh?: boolean;
  onAutoRefresh?: (on: boolean) => void;
}

export function SourceBanner({ meta, onReload, reloading, reloadError, autoRefresh, onAutoRefresh }: BannerProps) {
  const text = meta.asOf
    ? `AS RECORDED AT ${new Date(meta.asOf).toLocaleString()} · ${meta.label}`
    : meta.origin === 'live'
      ? `LIVE · ${meta.label}`
      : meta.origin === 'mock'
        ? 'DEMO DATA — synthetic, not live inventory'
        : `FILE — ${meta.label} (not live inventory)`;
  const at = meta.generatedAt ? new Date(meta.generatedAt) : null;
  return (
    <div className={`banner ${meta.asOf ? 'history' : meta.origin}`} title={meta.generatedAt ? `generated ${meta.generatedAt}` : undefined}>
      <div className="banner-row">
        <span>{text}</span>
        {onReload && (
          <button className="reload" onClick={onReload} disabled={reloading} title="Re-read the catalog; view state is kept">
            {reloading ? '…' : 'Reload'}
          </button>
        )}
      </div>
      <div className="banner-row banner-sub">
        {at && !Number.isNaN(at.getTime()) && <span>{meta.asOf ? `read ${at.toLocaleString()}` : `as of ${at.toLocaleString()}`}</span>}
        {autoRefresh !== undefined && onAutoRefresh && (
          <label className="auto" title="Reload when the catalog, rules or schemas change">
            <input type="checkbox" checked={autoRefresh} onChange={(e) => onAutoRefresh(e.target.checked)} /> auto
          </label>
        )}
      </div>
      {reloadError && <div className="banner-err">{reloadError}</div>}
    </div>
  );
}

export function Sidebar(props: Props) {
  const { index, views, projectionId, state, onState } = props;
  const view = views.get(projectionId)!;
  const [query, setQuery] = useState('');

  const localResults = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return [];
    return index.graph.resources
      .filter((r) => [r.name, r.nativeId, r.type, r.id].some((s) => s?.toLowerCase().includes(q)))
      .slice(0, 40);
  }, [index, query]);
  const [remoteResults, setRemoteResults] = useState<SearchHit[]>([]);
  const { remoteSearch } = props;
  useEffect(() => {
    if (!remoteSearch || !query.trim()) {
      setRemoteResults([]);
      return;
    }
    let cancelled = false;
    const timer = setTimeout(() => {
      remoteSearch(query.trim())
        .then((hits) => !cancelled && setRemoteResults(hits))
        .catch(() => !cancelled && setRemoteResults([]));
    }, 250);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [remoteSearch, query]);
  const results = remoteSearch ? remoteResults : localResults;
  const pick = (id: ResourceId) => (remoteSearch && props.onPickRemote ? props.onPickRemote(id) : props.onSelectResource(id));

  const nameOf = useMemo(() => (id: ResourceId) => index.resource(id)?.name ?? id, [index]);
  const inView = (id: ResourceId) => view.members.has(id) || view.groupOf.has(id);

  // Owners (accounts/projects) for the scope selector.
  const owners = useMemo(() => {
    const out = new Map<string, { provider: string; label: string }>();
    for (const r of index.graph.resources) {
      if (!r.account || out.has(r.account)) continue;
      const owner = ownerResource(index, r);
      out.set(r.account, { provider: r.provider, label: owner && owner.name !== r.account ? `${owner.name} (${r.account})` : r.account });
    }
    return [...out.entries()].sort((a, b) => a[1].provider.localeCompare(b[1].provider) || a[0].localeCompare(b[0]));
  }, [index]);

  // Resource types present in this view, with counts.
  const types = useMemo(() => {
    const counts = new Map<string, number>();
    const add = (id: ResourceId) => {
      const r = index.resource(id);
      if (r) counts.set(r.type, (counts.get(r.type) ?? 0) + 1);
    };
    for (const id of view.members.keys()) add(id);
    for (const id of view.groupOf.keys()) add(id);
    return [...counts.entries()].sort((a, b) => resourceType(a[0]).cls.localeCompare(resourceType(b[0]).cls) || a[0].localeCompare(b[0]));
  }, [index, view]);

  const toggleOwner = (owner: string) =>
    onState((s) => {
      const all = owners.map(([o]) => o);
      const scope = new Set(s.scope ?? all);
      if (scope.has(owner)) scope.delete(owner);
      else scope.add(owner);
      return { ...s, scope: scope.size === all.length ? null : scope };
    });
  const toggleType = (type: string) =>
    onState((s) => {
      const hiddenTypes = new Set(s.hiddenTypes);
      if (hiddenTypes.has(type)) hiddenTypes.delete(type);
      else hiddenTypes.add(type);
      return { ...s, hiddenTypes };
    });

  return (
    <aside className="sidebar">
      <div className="brand">
        PUDL <span>graph explorer</span>
      </div>
      <SourceBanner
        meta={props.meta}
        onReload={props.onReload}
        reloading={props.reloading}
        reloadError={props.reloadError}
        autoRefresh={props.autoRefresh}
        onAutoRefresh={props.onAutoRefresh}
      />
      {props.timeControl}

      <section>
        <h3>Views</h3>
        <div className="views">
          {PROJECTIONS.map((p) => (
            <button key={p.id} className={p.id === projectionId ? 'on' : ''} title={p.description} onClick={() => props.onProjection(p.id)}>
              {p.label}
            </button>
          ))}
        </div>
      </section>

      <section>
        <h3>{remoteSearch ? 'Search whole inventory' : 'Search inventory'}</h3>
        <input className="search" placeholder="name, ID, type…" value={query} onChange={(e) => setQuery(e.target.value)} />
        {results.length > 0 && (
          <ul className="results">
            {results.map((r) => (
              <li key={r.id} className={inView(r.id) || remoteSearch ? '' : 'out'} onClick={() => pick(r.id)} title={inView(r.id) ? r.id : remoteSearch ? `${r.id} — click to load it and its neighbours` : `${r.id} — not in this view`}>
                <span className={`code cls-${resourceType(r.type).cls}`}>{resourceType(r.type).code}</span>
                {r.name}
              </li>
            ))}
          </ul>
        )}
      </section>

      <section>
        <h3>Scope</h3>
        <ul className="checks">
          {owners.map(([owner, info]) => (
            <li key={owner}>
              <label>
                <input type="checkbox" checked={!state.scope || state.scope.has(owner)} onChange={() => toggleOwner(owner)} />
                <span className="prov">{info.provider.toUpperCase()}</span> {info.label}
              </label>
            </li>
          ))}
        </ul>
      </section>

      <section>
        <h3>Resource types</h3>
        <ul className="checks">
          {types.map(([type, n]) => (
            <li key={type}>
              <label title={type}>
                <input type="checkbox" checked={!state.hiddenTypes.has(type)} onChange={() => toggleType(type)} />
                <span className={`code cls-${resourceType(type).cls}`}>{resourceType(type).code}</span> {resourceType(type).label}
                <span className="count">{n}</span>
              </label>
            </li>
          ))}
        </ul>
      </section>

      <section className="grow">
        <h3>Hierarchy · {view.projection.label}</h3>
        <HierarchyTree view={view} state={state} selection={props.selection} nameOf={nameOf} onSelectResource={props.onSelectResource} onSelectGroup={props.onSelectGroup} onToggleGroup={props.onToggleGroup} />
      </section>
    </aside>
  );
}
