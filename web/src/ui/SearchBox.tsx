// Inventory search: ranked results with type and owner, keyboard navigation
// (↑/↓, Enter, Esc; "/" focuses the box from anywhere), and a result list
// that closes once a resource is picked.

import { useEffect, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from 'react';
import type { SearchHit } from '../data/source';
import type { GraphIndex, ResourceId } from '../model/canonical';
import { middleEllipsis } from '../render/text';
import { rankSearch } from './searchRank';

/** Characters of a result's name that fit the sidebar's width. */
const NAME_CHARS = 30;

interface Props {
  index: GraphIndex;
  /** Is the resource in the current view (local search only)? */
  inView(id: ResourceId): boolean;
  /** Partial graphs: search the whole inventory on the server. */
  remoteSearch?: (q: string) => Promise<SearchHit[]>;
  onPick(id: ResourceId): void;
}

export function SearchBox({ index, inView, remoteSearch, onPick }: Props) {
  const [query, setQuery] = useState('');
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const input = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLUListElement>(null);

  const localResults = useMemo(() => (remoteSearch ? [] : rankSearch(index.graph.resources, query, 40)), [index, query, remoteSearch]);
  const [remoteResults, setRemoteResults] = useState<SearchHit[]>([]);
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
  const results: SearchHit[] = remoteSearch ? remoteResults : localResults;
  useEffect(() => setActive(0), [results]);

  // "/" focuses the search box unless the user is typing elsewhere.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== '/' || e.metaKey || e.ctrlKey || e.altKey) return;
      if ((e.target as HTMLElement).closest?.('input, textarea, select')) return;
      e.preventDefault();
      input.current?.focus();
      input.current?.select();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  useEffect(() => {
    list.current?.querySelector<HTMLElement>('li.active')?.scrollIntoView({ block: 'nearest' });
  }, [active]);

  const pick = (id: ResourceId) => {
    setOpen(false);
    onPick(id);
  };
  const onKeyDown = (e: ReactKeyboardEvent) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      setOpen(true);
      const n = results.length;
      if (n) setActive((a) => (a + (e.key === 'ArrowDown' ? 1 : n - 1)) % n);
    } else if (e.key === 'Enter') {
      const hit = results[active];
      if (hit) pick(hit.id);
    } else if (e.key === 'Escape') {
      if (open && query) setOpen(false);
      else {
        setQuery('');
        input.current?.blur();
      }
    }
  };

  const showing = open && results.length > 0;
  return (
    <>
      <input
        ref={input}
        className="search"
        placeholder="name, ID, type…  ( / )"
        value={query}
        role="combobox"
        aria-expanded={showing}
        aria-controls="search-results"
        aria-activedescendant={showing ? `search-hit-${active}` : undefined}
        onChange={(e) => {
          setQuery(e.target.value);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        onKeyDown={onKeyDown}
      />
      {open && query.trim() && results.length === 0 && <div className="muted search-none">No matches.</div>}
      {showing && (
        <ul className="results" id="search-results" role="listbox" ref={list}>
          {results.map((r, i) => {
            const info = index.vocab.resourceType(r.type);
            const present = !!remoteSearch || inView(r.id);
            const where = [info.label, r.account].filter(Boolean).join(' · ');
            return (
              <li
                key={r.id}
                id={`search-hit-${i}`}
                role="option"
                aria-selected={i === active}
                className={`${present ? '' : 'out'}${i === active ? ' active' : ''}`}
                onMouseDown={(e) => e.preventDefault()}
                onMouseEnter={() => setActive(i)}
                onClick={() => pick(r.id)}
                title={`${r.name}\n${r.id}${remoteSearch ? '\nClick to load it and its neighbours' : present ? '' : '\nNot in this view'}`}
              >
                <div className="hit-name">
                  <span className={`code cls-${info.cls}`}>{info.code}</span>
                  {middleEllipsis(r.name, NAME_CHARS)}
                </div>
                <div className="hit-where">{where}</div>
              </li>
            );
          })}
        </ul>
      )}
    </>
  );
}
