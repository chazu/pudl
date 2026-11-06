// The sidebar's filter lists: scope (owners: accounts, projects) and the
// resource types in the current view. Each folds, summarises its filter,
// offers all/none, and gets a filter box when long.

import { useMemo, useState } from 'react';
import type { GraphIndex } from '../model/canonical';
import type { ViewState } from '../visual/viewState';
import { isOnlyOwner, isOnlyType, onlyOwner, onlyType } from './filters';
import { SidebarSection } from './SidebarSection';

/** Lists longer than this get a filter box. */
const FILTER_ABOVE = 8;
/** Lists longer than this get all/none buttons. */
const TOOLS_ABOVE = 3;

type OnState = (fn: (s: ViewState) => ViewState) => void;

export interface OwnerItem {
  owner: string;
  provider: string;
  label: string;
}

export function ScopeSection({ owners, state, onState }: { owners: OwnerItem[]; state: ViewState; onState: OnState }) {
  const [filter, setFilter] = useState('');
  const all = owners.map((o) => o.owner);
  const shown = matching(owners, filter, (o) => `${o.label} ${o.owner}`);
  const toggle = (owner: string) =>
    onState((s) => {
      const scope = new Set(s.scope ?? all);
      if (scope.has(owner)) scope.delete(owner);
      else scope.add(owner);
      return { ...s, scope: scope.size === all.length ? null : scope };
    });
  const inScope = state.scope ? all.filter((o) => state.scope!.has(o)).length : all.length;
  if (owners.length === 0) return null;
  return (
    <SidebarSection
      id="scope"
      title="Scope"
      summary={owners.length === 0 ? undefined : inScope === all.length ? `all ${all.length}` : `${inScope} of ${all.length}`}
      summaryActive={inScope !== all.length}
      defaultFolded={owners.length > FILTER_ABOVE}
    >
      {owners.length > TOOLS_ABOVE && (
        <ListTools
          filter={owners.length > FILTER_ABOVE ? filter : undefined}
          onFilter={setFilter}
          placeholder="filter accounts and projects"
          onAll={() => onState((s) => ({ ...s, scope: null }))}
          onNone={() => onState((s) => ({ ...s, scope: new Set() }))}
        />
      )}
      <ul className="checks">
        {shown.map((o) => (
          <li key={o.owner}>
            <label title={`${o.label}${o.label.includes(o.owner) ? '' : ` (${o.owner})`}`}>
              <input type="checkbox" checked={!state.scope || state.scope.has(o.owner)} onChange={() => toggle(o.owner)} />
              <span className="prov">{o.provider.toUpperCase()}</span> <span className="item-label">{o.label}</span>
            </label>
            <OnlyButton only={isOnlyOwner(state.scope, o.owner)} what={o.label} onClick={() => onState((s) => ({ ...s, scope: onlyOwner(s.scope, o.owner) }))} />
          </li>
        ))}
      </ul>
    </SidebarSection>
  );
}

interface TypeProps {
  index: GraphIndex;
  /** Types in the current view, with counts. */
  types: [string, number][];
  state: ViewState;
  onState: OnState;
}

export function TypeSection({ index, types, state, onState }: TypeProps) {
  const [filter, setFilter] = useState('');
  // Every type in the graph, so "only" and "none" hold when the view changes.
  const allTypes = useMemo(() => [...new Set(index.graph.resources.map((r) => r.type))], [index]);
  const info = (type: string) => index.vocab.resourceType(type);
  const shown = matching(types, filter, ([type]) => `${info(type).label} ${type}`);
  const toggle = (type: string) =>
    onState((s) => {
      const hiddenTypes = new Set(s.hiddenTypes);
      if (hiddenTypes.has(type)) hiddenTypes.delete(type);
      else hiddenTypes.add(type);
      return { ...s, hiddenTypes };
    });
  const visible = types.filter(([t]) => !state.hiddenTypes.has(t)).length;
  return (
    <SidebarSection
      id="types"
      title="Resource types"
      summary={types.length === 0 ? undefined : visible === types.length ? `all ${types.length}` : `${visible} of ${types.length}`}
      summaryActive={visible !== types.length}
    >
      {types.length > TOOLS_ABOVE && (
        <ListTools
          filter={types.length > FILTER_ABOVE ? filter : undefined}
          onFilter={setFilter}
          placeholder="filter types"
          onAll={() => onState((s) => ({ ...s, hiddenTypes: new Set() }))}
          onNone={() => onState((s) => ({ ...s, hiddenTypes: new Set(allTypes) }))}
        />
      )}
      <ul className="checks">
        {shown.map(([type, n]) => (
          <li key={type}>
            <label title={type}>
              <input type="checkbox" checked={!state.hiddenTypes.has(type)} onChange={() => toggle(type)} />
              <span className={`code cls-${info(type).cls}`}>{info(type).code}</span> <span className="item-label">{info(type).label}</span>
              <span className="count">{n}</span>
            </label>
            <OnlyButton only={isOnlyType(state.hiddenTypes, type, allTypes)} what={info(type).label} onClick={() => onState((s) => ({ ...s, hiddenTypes: onlyType(s.hiddenTypes, type, allTypes) }))} />
          </li>
        ))}
      </ul>
    </SidebarSection>
  );
}

function matching<T>(items: T[], filter: string, text: (item: T) => string): T[] {
  const q = filter.trim().toLowerCase();
  return q ? items.filter((i) => text(i).toLowerCase().includes(q)) : items;
}

interface ToolsProps {
  /** Undefined: no filter box. */
  filter?: string;
  onFilter(v: string): void;
  placeholder: string;
  onAll(): void;
  onNone(): void;
}

function ListTools({ filter, onFilter, placeholder, onAll, onNone }: ToolsProps) {
  return (
    <div className="list-tools">
      {filter !== undefined && (
        <input className="list-filter" placeholder={placeholder} value={filter} onChange={(e) => onFilter(e.target.value)} onKeyDown={(e) => e.key === 'Escape' && onFilter('')} />
      )}
      <button type="button" onClick={onAll} title="Show all">
        all
      </button>
      <button type="button" onClick={onNone} title="Hide all">
        none
      </button>
    </div>
  );
}

/** Isolates one list item; reads "all" (and shows everything) once it is isolated. */
function OnlyButton({ only, what, onClick }: { only: boolean; what: string; onClick(): void }) {
  return (
    <button type="button" className={`only${only ? ' on' : ''}`} onClick={onClick} title={only ? 'Show all again' : `Show only ${what}`}>
      {only ? 'all' : 'only'}
    </button>
  );
}
