import type { InspectorProps } from './Inspector';
import { BasisBadge, EvidenceList, LineSample, PropertyTable } from './inspectorParts';
import type { Basis } from '../model/canonical';

export function EdgePanel(props: InspectorProps & { edgeId: string }) {
  const { edgeId, graph, index } = props;
  const edge = graph?.edges.find((e) => e.id === edgeId);
  if (!edge || !graph) return <div className="panel muted">Edge no longer drawn.</div>;
  const cat = index.vocab.category(edge.category);
  const src = graph.nodes.get(edge.source);
  const tgt = graph.nodes.get(edge.target);
  const lifted = edge.liftedSource || edge.liftedTarget;

  return (
    <div className="panel">
      <div className="phead">
        <LineSample category={edge.category} />
        <h2>{edge.aggregated ? `${edge.relationshipIds.length} relationships` : index.vocab.relationshipType(edge.types[0]).label}</h2>
      </div>
      <div className="ptype">
        {cat.label} · {edge.types.join(', ')}
      </div>
      <div className="notice">{cat.meaning}</div>

      <dl className="kv">
        <dt>From</dt>
        <dd>
          {src?.label}
          {edge.liftedSource && <span className="muted"> (collapsed group boundary)</span>}
        </dd>
        <dt>To</dt>
        <dd>
          {tgt?.label}
          {edge.liftedTarget && <span className="muted"> (collapsed group boundary)</span>}
        </dd>
        <dt>Basis</dt>
        <dd>
          {(Object.entries(edge.basis) as [Basis, number][]).map(([b, n]) => (
            <span key={b}>
              <BasisBadge basis={b} /> {n}{' '}
            </span>
          ))}
        </dd>
      </dl>
      {lifted && (
        <div className="notice">
          At least one end sits inside a collapsed group, so the edge ends at the group’s boundary port. The boundary is
          a drawing device, not a network hop. The real endpoints are listed below.
        </div>
      )}

      <h3>Underlying relationships</h3>
      <ul className="underlying">
        {edge.relationshipIds.map((rid) => {
          const rel = index.relationship(rid);
          if (!rel) return null;
          const info = index.vocab.relationshipType(rel.type);
          return (
            <li key={rid}>
              <div className="relrow">
                <LineSample category={info.category} />
                <strong>{info.label}</strong>
                <BasisBadge basis={rel.basis} />
              </div>
              <div className="endpoints">
                <span className="peer" onClick={() => props.onSelectResource(rel.source)}>{index.resource(rel.source)?.name ?? `${rel.source} (not in inventory)`}</span>
                <span className="arrow">→</span>
                <span className="peer" onClick={() => props.onSelectResource(rel.target)}>{index.resource(rel.target)?.name ?? `${rel.target} (not in inventory)`}</span>
              </div>
              <div className="muted">{info.meaning}</div>
              {rel.properties && <PropertyTable props={rel.properties} />}
              <EvidenceList evidence={rel.evidence} />
            </li>
          );
        })}
      </ul>
    </div>
  );
}
