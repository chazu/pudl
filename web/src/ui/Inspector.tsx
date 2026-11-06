// Right-hand inspector: routes the selection to a resource, group or edge panel.

import type { GraphIndex, ResourceId } from '../model/canonical';
import type { ProjectionId, SemanticView } from '../projection/types';
import type { Selection } from '../render/highlight';
import type { VisualGraph } from '../visual/types';
import type { ViewState } from '../visual/viewState';
import { EdgePanel } from './EdgePanel';
import { GroupPanel, ResourcePanel } from './ResourcePanel';

export interface InspectorProps {
  index: GraphIndex;
  views: Map<ProjectionId, SemanticView>;
  projectionId: ProjectionId;
  graph: VisualGraph | null;
  state: ViewState;
  selection: Selection;
  onSelectResource(id: ResourceId): void;
  onSelectEdge(id: string): void;
  onProjection(id: ProjectionId): void;
  onToggleGroup(groupId: string): void;
  onFocus(id: ResourceId, depth: number): void;
  /** Partial graphs: load a resource's neighbours. */
  onExpand?(id: ResourceId): void;
}

interface Props extends InspectorProps {
  open: boolean;
  onToggle(): void;
}

export function Inspector({ open, onToggle, ...props }: Props) {
  const { selection } = props;
  if (!open) {
    return (
      <aside className="inspector closed">
        <button type="button" className="insp-toggle" title="Show the inspector (i)" onClick={onToggle}>
          «
        </button>
      </aside>
    );
  }
  return (
    <aside className="inspector">
      <button type="button" className="insp-toggle insp-hide" title="Hide the inspector (i)" onClick={onToggle}>
        »
      </button>
      {!selection && <EmptyInspector />}
      {selection?.kind === 'resource' && <ResourcePanel {...props} id={selection.id} />}
      {selection?.kind === 'group' && <GroupPanel {...props} groupId={selection.id} />}
      {selection?.kind === 'edge' && <EdgePanel {...props} edgeId={selection.id} />}
    </aside>
  );
}

function EmptyInspector() {
  return (
    <div className="panel empty">
      <h2>Inspector</h2>
      <p>Select a resource, group or edge.</p>
      <dl className="help">
        <dt>Drag</dt>
        <dd>pan</dd>
        <dt>Wheel</dt>
        <dd>zoom</dd>
        <dt>Double-click group</dt>
        <dd>collapse / expand</dd>
        <dt>f / Esc</dt>
        <dd>fit / clear selection</dd>
        <dt>/</dt>
        <dd>search</dd>
        <dt>i</dt>
        <dd>hide / show this panel</dd>
      </dl>
      <p className="muted">
        Edge styles distinguish categories. Nesting is a property of the current view, not a claim of exclusive
        ownership. Dashed outlines are groupings, not resources.
      </p>
    </div>
  );
}
