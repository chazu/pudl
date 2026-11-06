import { useMemo, type ReactNode } from 'react';
import type { SearchHit } from '../data/source';
import type { GraphIndex, GraphMeta, ResourceId } from '../model/canonical';
import { PROJECTIONS } from '../projection/registry';
import { ownerResource } from '../projection/views/common';
import type { ProjectionId, SemanticView } from '../projection/types';
import type { Selection } from '../render/highlight';
import type { ViewState } from '../visual/viewState';
import { CoveragePanel } from './CoveragePanel';
import { ScopeSection, TypeSection, type OwnerItem } from './FilterLists';
import { HierarchyTree } from './HierarchyTree';
import { SearchBox } from './SearchBox';
import { SidebarSection } from './SidebarSection';

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
  const nameOf = useMemo(() => (id: ResourceId) => index.resource(id)?.name ?? id, [index]);
  const inView = (id: ResourceId) => view.members.has(id) || view.groupOf.has(id);

  // Owners (accounts/projects) for the scope selector.
  const owners = useMemo(() => {
    const out = new Map<string, OwnerItem>();
    for (const r of index.graph.resources) {
      if (!r.account || out.has(r.account)) continue;
      const owner = ownerResource(index, r);
      out.set(r.account, { owner: r.account, provider: r.provider, label: owner && owner.name !== r.account ? `${owner.name} (${r.account})` : r.account });
    }
    return [...out.values()].sort((a, b) => a.provider.localeCompare(b.provider) || a.owner.localeCompare(b.owner));
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
    return [...counts.entries()].sort((a, b) => index.vocab.resourceType(a[0]).cls.localeCompare(index.vocab.resourceType(b[0]).cls) || a[0].localeCompare(b[0]));
  }, [index, view]);

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
      {props.meta.coverage && <CoveragePanel coverage={props.meta.coverage} />}

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
        <h3>{props.remoteSearch ? 'Search whole inventory' : 'Search inventory'}</h3>
        <SearchBox
          index={index}
          inView={inView}
          remoteSearch={props.remoteSearch}
          onPick={(id) => (props.remoteSearch && props.onPickRemote ? props.onPickRemote(id) : props.onSelectResource(id))}
        />
      </section>

      <ScopeSection owners={owners} state={state} onState={onState} />
      <TypeSection index={index} types={types} state={state} onState={onState} />

      <SidebarSection id="hierarchy" title={`Hierarchy · ${view.projection.label}`} grow>
        <HierarchyTree view={view} state={state} selection={props.selection} nameOf={nameOf} onSelectResource={props.onSelectResource} onSelectGroup={props.onSelectGroup} onToggleGroup={props.onToggleGroup} />
      </SidebarSection>
    </aside>
  );
}
