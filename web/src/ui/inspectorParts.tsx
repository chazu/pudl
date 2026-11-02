// Building blocks shared by the inspector panels.

import type { Basis, Evidence, GraphIndex, Relationship, ResourceId } from '../model/canonical';
import { relationshipType } from '../model/relationshipTypes';
import type { SemanticView } from '../projection/types';
import { EDGE_STYLES } from '../render/edgeStyle';
import type { VisualGraph } from '../visual/types';

export function BasisBadge({ basis }: { basis: Basis }) {
  const titles: Record<Basis, string> = {
    observed: 'Seen in a collector snapshot',
    declared: 'Stated by configuration or a manifest; not observed',
    inferred: 'Derived by a rule or heuristic; weakest evidence',
    unknown: 'No provenance available',
  };
  return <span className={`basis basis-${basis}`} title={titles[basis]}>{basis}</span>;
}

export function LineSample({ type }: { type: string }) {
  const st = EDGE_STYLES[relationshipType(type).category];
  return (
    <svg className="linesample" width="22" height="8">
      <line x1="1" y1="4" x2="21" y2="4" stroke={st.color} strokeWidth={st.width + 0.5} strokeDasharray={st.dash} />
    </svg>
  );
}

export interface DispositionInfo {
  text: string;
  edgeId?: string;
  drawn: boolean;
}

/** How a relationship appears (or why it doesn't) in the current view. */
export function disposition(relId: string, view: SemanticView, graph: VisualGraph | null): DispositionInfo {
  const p = view.dispositions.get(relId);
  if (!p) return { text: 'unknown', drawn: false };
  if (p.kind === 'port-binding') return { text: 'shown as interface on host', drawn: true };
  if (p.kind === 'category-not-in-view') return { text: 'category not part of this view', drawn: false };
  if (p.kind === 'endpoint-not-in-view') return { text: 'other end not in this view', drawn: false };
  const v = graph?.dispositions.get(relId);
  if (!v) return { text: 'pending layout', drawn: false };
  switch (v.kind) {
    case 'drawn': {
      const e = graph!.edges.find((x) => x.id === v.edgeId);
      return { text: e?.aggregated ? `drawn, in ${e.relationshipIds.length}× edge` : 'drawn', edgeId: v.edgeId, drawn: true };
    }
    case 'internal':
      return { text: `inside collapsed ${graph!.nodes.get(v.groupNodeId)?.label ?? 'group'}`, drawn: false };
    case 'containment':
      return { text: 'shown as nesting', drawn: true };
    case 'to-enclosing-group':
      return { text: 'targets its enclosing group; not drawn', drawn: false };
    case 'category-off':
      return { text: 'category filtered out', drawn: false };
    case 'endpoint-hidden':
      return { text: `hidden: ${v.reason}`, drawn: false };
  }
}

interface RelListProps {
  title: string;
  rels: Relationship[];
  self: ResourceId;
  index: GraphIndex;
  view: SemanticView;
  graph: VisualGraph | null;
  onSelectResource(id: ResourceId): void;
  onSelectEdge(id: string): void;
}

export function RelationshipList({ title, rels, self, index, view, graph, onSelectResource, onSelectEdge }: RelListProps) {
  if (rels.length === 0) return null;
  const sorted = [...rels].sort((a, b) => relationshipType(a.type).category.localeCompare(relationshipType(b.type).category) || a.type.localeCompare(b.type));
  return (
    <div className="relsection">
      <h4>
        {title} <span className="count">{rels.length}</span>
      </h4>
      <ul className="rels">
        {sorted.map((rel) => {
          const peer = rel.source === self ? rel.target : rel.source;
          const peerRes = index.resource(peer);
          const d = disposition(rel.id, view, graph);
          const info = relationshipType(rel.type);
          return (
            <li key={rel.id} className={d.drawn ? '' : 'undrawn'}>
              <div className="relrow">
                <LineSample type={rel.type} />
                <span className="reltype" title={info.meaning}>{info.label}</span>
                <span className={`peer${peerRes ? '' : ' missing'}`} onClick={() => onSelectResource(peer)} title={peer}>
                  {peerRes?.name ?? `${peer} (not in inventory)`}
                </span>
                <BasisBadge basis={rel.basis} />
              </div>
              <div className="relmeta">
                {d.edgeId ? <a onClick={() => onSelectEdge(d.edgeId!)}>{d.text}</a> : d.text}
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

export function EvidenceList({ evidence }: { evidence?: Evidence[] }) {
  if (!evidence || evidence.length === 0) return <div className="muted">No provenance recorded.</div>;
  return (
    <ul className="evidence">
      {evidence.map((e, i) => (
        <li key={i}>
          <BasisBadge basis={e.basis} /> <code>{e.source}</code>
          {e.detail && <div className="muted">{e.detail}</div>}
          {(e.observedAt || e.snapshotId) && (
            <div className="muted">
              {e.observedAt}
              {e.snapshotId ? ` · snapshot ${e.snapshotId}` : ''}
            </div>
          )}
        </li>
      ))}
    </ul>
  );
}

export function PropertyTable({ props }: { props: Record<string, unknown> }) {
  const entries = Object.entries(props);
  if (entries.length === 0) return null;
  return (
    <table className="props">
      <tbody>
        {entries.map(([k, v]) => (
          <tr key={k}>
            <th>{k}</th>
            <td>{typeof v === 'string' ? v : JSON.stringify(v)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
