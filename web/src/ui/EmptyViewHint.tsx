// Shown on an empty canvas when resources are loaded but this view draws
// none of them: says why, and offers the views that do show something (or
// the selected resource).

import type { ResourceId } from '../model/canonical';
import { PROJECTIONS } from '../projection/registry';
import type { ProjectionDefinition, ProjectionId, SemanticView } from '../projection/types';

export function inView(view: SemanticView | undefined, id: ResourceId): boolean {
  return !!view && (view.members.has(id) || view.groupOf.has(id));
}

interface Props {
  view: SemanticView;
  views: Map<ProjectionId, SemanticView>;
  loadedCount: number;
  partial: boolean;
  /** Name of the selected resource, if one is selected. */
  selected?: string;
  /** Views the selected resource appears in. */
  selectedIn: ProjectionDefinition[];
  onProjection(id: ProjectionId): void;
}

export function EmptyViewHint({ view, views, loadedCount, partial, selected, selectedIn, onProjection }: Props) {
  const here = view.projection.id;
  const others = selected
    ? selectedIn.filter((p) => p.id !== here)
    : PROJECTIONS.filter((p) => p.id !== here && (views.get(p.id)?.members.size ?? 0) > 0);
  const what = partial ? `${loadedCount} loaded resource(s)` : `${loadedCount} resources`;
  return (
    <div className="canvas-hint">
      <div className="interactive">
        <strong>
          {selected ? `${selected} is not in the ${view.projection.label} view` : `This view shows none of the ${what}.`}
        </strong>{' '}
        {view.projection.description}
        {partial && !selected && ' Load more from search or a ⋯ badge, or switch view.'}
        {others.length > 0 && (
          <div className="actions">
            {others.map((p) => (
              <button key={p.id} onClick={() => onProjection(p.id)}>
                {p.label}
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
