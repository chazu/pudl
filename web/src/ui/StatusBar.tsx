import type { GraphIssue } from '../data/validate';
import type { SemanticView } from '../projection/types';
import type { Rendered } from './usePipeline';

interface Props {
  rendered: Rendered | null;
  view: SemanticView;
  issues: GraphIssue[];
  engineName: string;
}

export function StatusBar({ rendered, view, issues, engineName }: Props) {
  if (!rendered) return <div className="statusbar">—</div>;
  const { graph, layout } = rendered;
  const counts: Record<string, number> = {};
  for (const d of graph.dispositions.values()) counts[d.kind] = (counts[d.kind] ?? 0) + 1;
  const aggregated = graph.edges.filter((e) => e.aggregated).length;
  const hidden = (counts['category-off'] ?? 0) + (counts['endpoint-hidden'] ?? 0);
  const dangling = issues.filter((i) => i.kind === 'dangling-source' || i.kind === 'dangling-target').length;
  return (
    <div className="statusbar">
      <span>{graph.nodes.size} nodes</span>
      <span>
        {graph.edges.length} edges ({counts.drawn ?? 0} relationships, {aggregated} aggregated)
      </span>
      <span title="Relationships expressed by nesting rather than an edge">{counts.containment ?? 0} as nesting</span>
      <span title="Relationships with both ends inside a collapsed group">{counts.internal ?? 0} inside collapsed</span>
      <span title="Relationships hidden by category or resource filters">{hidden} filtered</span>
      <span>{view.excluded.length} resources not in this view</span>
      {dangling > 0 && <span className="warn">{dangling} unresolved reference(s)</span>}
      <span className="right">
        {engineName} · {Math.round(layout.durationMs)} ms
      </span>
    </div>
  );
}
