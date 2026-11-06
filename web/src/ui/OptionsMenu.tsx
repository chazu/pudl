// The toolbar's less-used layout and drawing options, in a dropdown. A dot on
// the button shows when any differs from the view's default.

import { useEffect, useRef, useState } from 'react';
import type { SemanticView } from '../projection/types';
import type { ViewState } from '../visual/viewState';

interface Props {
  view: SemanticView;
  state: ViewState;
  onState(fn: (s: ViewState) => ViewState): void;
}

export function OptionsMenu({ view, state, onState }: Props) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent | KeyboardEvent) => {
      if (e instanceof KeyboardEvent ? e.key === 'Escape' : !root.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', close);
    document.addEventListener('keydown', close);
    return () => {
      document.removeEventListener('mousedown', close);
      document.removeEventListener('keydown', close);
    };
  }, [open]);

  const layered = view.projection.layout.algorithm === 'layered';
  const changed =
    state.bundleParallel || state.showUnresolved || state.reflowOnFilter || (layered && state.direction !== view.projection.layout.direction);

  return (
    <div className="tb-menu" ref={root}>
      <button className={open ? 'on' : ''} aria-expanded={open} onClick={() => setOpen(!open)} title="Layout and drawing options">
        Options{changed ? ' •' : ''} ▾
      </button>
      {open && (
        <div className="tb-menu-body" role="menu">
          {layered && (
            <label className="tb-check" title="Layout direction">
              <span>Direction</span>
              <button onClick={() => onState((s) => ({ ...s, direction: s.direction === 'RIGHT' ? 'DOWN' : 'RIGHT' }))}>
                {state.direction === 'RIGHT' ? 'Left → right' : 'Top → bottom'}
              </button>
            </label>
          )}
          <label className="tb-check" title="Merge parallel edges of the same category between the same endpoints">
            <input type="checkbox" checked={state.bundleParallel} onChange={(e) => onState((s) => ({ ...s, bundleParallel: e.target.checked }))} />
            Bundle parallel edges
          </label>
          <label className="tb-check" title="Draw resources that relationships name but the inventory lacks, in a 'Not in inventory' group">
            <input type="checkbox" checked={state.showUnresolved} onChange={(e) => onState((s) => ({ ...s, showUnresolved: e.target.checked }))} />
            Show unresolved references
          </label>
          <label className="tb-check" title="Off: filters hide things in place and nothing moves. On: filters re-run layout to close gaps.">
            <input type="checkbox" checked={state.reflowOnFilter} onChange={(e) => onState((s) => ({ ...s, reflowOnFilter: e.target.checked }))} />
            Reflow on filter
          </label>
        </div>
      )}
    </div>
  );
}
