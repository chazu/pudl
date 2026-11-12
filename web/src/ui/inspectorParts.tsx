// Building blocks shared by the inspector panels.

import { useState } from 'react';
import type { Basis, Detail, Evidence, GraphIndex, Relationship, Representation, ResourceId } from '../model/canonical';
import { groupDetails } from '../model/details';
import type { RelationshipCategory } from '../model/vocabulary';
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

export function LineSample({ category }: { category: RelationshipCategory }) {
  const st = EDGE_STYLES[category];
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
  const categoryOf = (type: string) => index.vocab.relationshipType(type).category;
  const sorted = [...rels].sort((a, b) => categoryOf(a.type).localeCompare(categoryOf(b.type)) || a.type.localeCompare(b.type));
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
          const info = index.vocab.relationshipType(rel.type);
          return (
            <li key={rel.id} className={d.drawn ? '' : 'undrawn'}>
              <div className="relrow">
                <LineSample category={info.category} />
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

/** The cataloged records that describe one resource, one per schema. */
export function RepresentationList({ reps }: { reps: Representation[] }) {
  return (
    <ul className="representations">
      {reps.map((rep) => (
        <li key={rep.id}>
          <code className="wrap">{rep.schema}</code> <span className="muted">from {rep.origin || 'unknown origin'}</span>
          <div className="muted" title="Inspect the record with pudl show">
            <code className="wrap">pudl show {rep.entryId}</code>
          </div>
        </li>
      ))}
    </ul>
  );
}

/** Sections of graph_detail rows: one heading per section, a table of keys. */
export function DetailSections({ details }: { details: Detail[] | undefined }) {
  const sections = groupDetails(details);
  if (sections.length === 0) return null;
  return (
    <>
      {sections.map((s) => (
        <div key={s.section}>
          <h3>{s.section}</h3>
          <table className="props">
            <tbody>
              {s.rows.map((r) => (
                <tr key={r.key}>
                  <th>{r.key}</th>
                  <td>{r.values.join(r.ranked ? ' › ' : ', ')}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ))}
    </>
  );
}

/** Copies `text` to the clipboard; confirms briefly. */
export function CopyButton({ text }: { text: string }) {
  const [done, setDone] = useState(false);
  return (
    <button
      type="button"
      className="copy"
      title="Copy"
      onClick={() => {
        navigator.clipboard?.writeText(text).then(
          () => {
            setDone(true);
            setTimeout(() => setDone(false), 1200);
          },
          () => {},
        );
      }}
    >
      {done ? 'copied' : 'copy'}
    </button>
  );
}

export interface IdentifierRow {
  label: string;
  value?: string;
  title?: string;
}

/** Long identifiers, folded away by default, each with a copy button. */
export function Identifiers({ ids }: { ids: IdentifierRow[] }) {
  const rows = ids.filter((i): i is IdentifierRow & { value: string } => !!i.value);
  if (rows.length === 0) return null;
  return (
    <details className="ids">
      <summary>Identifiers</summary>
      <dl className="kv">
        {rows.map((row) => (
          <div key={row.label} className="kv-row">
            <dt>{row.label}</dt>
            <dd title={row.title}>
              <code className="wrap">{row.value}</code> <CopyButton text={row.value} />
            </dd>
          </div>
        ))}
      </dl>
    </details>
  );
}
