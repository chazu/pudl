import type { RelationshipCategory } from '../model/vocabulary';
import { OptionsMenu } from './OptionsMenu';
import type { SemanticView } from '../projection/types';
import { EDGE_STYLES } from '../render/edgeStyle';
import type { Selection } from '../render/highlight';
import type { ViewState } from '../visual/viewState';

interface Props {
  view: SemanticView;
  state: ViewState;
  selection: Selection;
  pending: boolean;
  onState(fn: (s: ViewState) => ViewState): void;
  onFit(): void;
  onZoom(factor: number): void;
  onRelayout(): void;
}

export function Toolbar({ view, state, selection, pending, onState, onFit, onZoom, onRelayout }: Props) {
  const available = view.vocab.categoryOrder().filter((c) => view.projection.categories.available.includes(c));
  const toggleCategory = (c: RelationshipCategory) =>
    onState((s) => {
      const enabledCategories = new Set(s.enabledCategories);
      if (enabledCategories.has(c)) enabledCategories.delete(c);
      else enabledCategories.add(c);
      return { ...s, enabledCategories };
    });
  const focusTarget = selection?.kind === 'resource' ? selection.id : null;
  const groupIds = [...view.groups.keys()];

  return (
    <div className="toolbar">
      <div className="tb-group">
        <button title="Zoom to fit (f)" onClick={onFit}>Fit</button>
        <button title="Zoom out (-)" onClick={() => onZoom(0.8)}>−</button>
        <button title="Zoom in (+)" onClick={() => onZoom(1.25)}>+</button>
        <button title="Discard remembered positions and lay out from scratch" onClick={onRelayout}>Relayout</button>
      </div>
      <div className="tb-group">
        <button onClick={() => onState((s) => ({ ...s, collapsed: new Set() }))}>Expand all</button>
        <button onClick={() => onState((s) => ({ ...s, collapsed: new Set(groupIds) }))}>Collapse all</button>
      </div>
      <div className="tb-group" title="Show only the selected resource and its neighbourhood over enabled relationships">
        <span className="tb-label">Focus</span>
        {[0, 1, 2].map((d) => (
          <button
            key={d}
            className={(state.focus?.depth ?? 0) === d ? 'on' : ''}
            disabled={d > 0 && !focusTarget && !state.focus}
            onClick={() =>
              onState((s) => ({
                ...s,
                focus: d === 0 ? null : { resourceId: focusTarget ?? s.focus!.resourceId, depth: d },
              }))
            }
          >
            {d === 0 ? 'Off' : `${d} hop`}
          </button>
        ))}
      </div>
      <div className="tb-group">
        <OptionsMenu view={view} state={state} onState={onState} />
      </div>
      <div className="tb-group legend">
        {available.map((c) => {
          const st = EDGE_STYLES[c];
          return (
            <button key={c} className={`chip${state.enabledCategories.has(c) ? ' on' : ''}`} title={view.vocab.category(c).meaning} onClick={() => toggleCategory(c)}>
              <svg width="26" height="8">
                <line x1="1" y1="4" x2="25" y2="4" stroke={st.color} strokeWidth={st.width + 0.5} strokeDasharray={st.dash} />
              </svg>
              {view.vocab.category(c).label}
            </button>
          );
        })}
      </div>
      {pending && <span className="tb-pending">layout…</span>}
    </div>
  );
}
