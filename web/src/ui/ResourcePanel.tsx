import { notLoaded } from '../data/merge';
import { groupAncestors } from '../projection/project';
import { PROJECTIONS } from '../projection/registry';
import type { InspectorProps } from './Inspector';
import { EvidenceList, PropertyTable, RelationshipList, disposition } from './inspectorParts';
import type { ResourceId } from '../model/canonical';

export function ResourcePanel(props: InspectorProps & { id: ResourceId }) {
  const { id, index, views, projectionId, graph, state } = props;
  const view = views.get(projectionId)!;
  const r = index.resource(id);
  const out = index.out(id);
  const inc = index.in(id);
  const relProps = { self: id, index, view, graph, onSelectResource: props.onSelectResource, onSelectEdge: props.onSelectEdge };

  if (!r) {
    return (
      <div className="panel">
        <div className="phead">
          <span className="code cls-unresolved">?</span>
          <h2>{id}</h2>
        </div>
        <div className="notice">
          Not in the inventory. PUDL holds only the reference below; nothing is known about this resource’s type, owner
          or connections, and none is assumed.
        </div>
        <RelationshipList title="Referenced by" rels={[...out, ...inc]} {...relProps} />
      </div>
    );
  }

  const info = index.vocab.resourceType(r.type);
  const member = view.members.get(id);
  const groupId = view.groupOf.get(id);
  const parentId = member?.parentId ?? (groupId ? view.groups.get(groupId)?.parentId : null) ?? null;
  const path = groupAncestors(view.groups, parentId).reverse().map((g) => view.groups.get(g)!.label);
  const nodeId = graph?.resourceToNode.get(id);
  const shownAs = nodeId ? graph?.nodes.get(nodeId) : undefined;
  const hiddenInside = shownAs && shownAs.resourceId !== id && member?.role !== 'port';
  const inView = !!member || !!groupId;

  return (
    <div className="panel">
      <div className="phead">
        <span className={`code cls-${info.cls}`}>{info.code}</span>
        <h2>{r.name}</h2>
      </div>
      <div className="ptype">
        {info.label} · <code>{r.type}</code>
      </div>

      <dl className="kv">
        <dt>Provider</dt>
        <dd>{r.provider}</dd>
        {r.nativeId && (
          <>
            <dt>Native ID</dt>
            <dd><code className="wrap">{r.nativeId}</code></dd>
          </>
        )}
        {r.account && (
          <>
            <dt>{r.provider === 'gcp' ? 'Project' : 'Account'}</dt>
            <dd>{r.account}</dd>
          </>
        )}
        {r.region && (
          <>
            <dt>Region</dt>
            <dd>{r.region}{r.zone ? ` / ${r.zone}` : ''}</dd>
          </>
        )}
        <dt>PUDL ID</dt>
        <dd><code className="wrap">{r.id}</code></dd>
        {r.schema && (
          <>
            <dt>Schema</dt>
            <dd><code className="wrap">{r.schema}</code></dd>
          </>
        )}
        {r.entryId && (
          <>
            <dt>Entry</dt>
            <dd title="Inspect the record with pudl show"><code className="wrap">pudl show {r.entryId}</code></dd>
          </>
        )}
      </dl>

      {props.onExpand && notLoaded(index, id) > 0 && (
        <div className="notice">
          {notLoaded(index, id)} relationship(s) of this resource lead to resources not loaded yet.
          <div className="actions">
            <button onClick={() => props.onExpand!(id)}>Load neighbours</button>
          </div>
        </div>
      )}

      <h3>In this view</h3>
      {!inView && <div className="muted">Not part of the {view.projection.label} view.</div>}
      {inView && (
        <>
          <div className="path">{path.length ? path.join(' / ') : 'top level'}</div>
          <div className="muted">{member?.placementBasis ?? view.groups.get(groupId!)?.basis}</div>
          {member?.placementNote && <div className="notice">{member.placementNote}</div>}
          {hiddenInside && <div className="notice">Inside collapsed group “{shownAs!.label}”.</div>}
          {!nodeId && state && <div className="notice">Hidden by the current filters or focus.</div>}
          <div className="actions">
            {groupId && <button onClick={() => props.onToggleGroup(groupId)}>{state.collapsed.has(groupId) ? 'Expand group' : 'Collapse group'}</button>}
            <button onClick={() => props.onFocus(id, 1)}>Focus 1 hop</button>
            <button onClick={() => props.onFocus(id, 2)}>Focus 2 hops</button>
            {state.focus && <button onClick={() => props.onFocus(id, 0)}>Clear focus</button>}
          </div>
        </>
      )}

      <h3>Appears in</h3>
      <div className="appears">
        {PROJECTIONS.map((p) => {
          const v = views.get(p.id)!;
          const present = v.members.has(id) || v.groupOf.has(id);
          return (
            <button key={p.id} className={`${present ? 'yes' : 'no'}${p.id === projectionId ? ' on' : ''}`} disabled={!present} onClick={() => props.onProjection(p.id)}>
              {present ? '●' : '○'} {p.label}
            </button>
          );
        })}
      </div>

      <h3>Relationships</h3>
      <RelationshipList title="Outbound" rels={out} {...relProps} />
      <RelationshipList title="Inbound" rels={inc} {...relProps} />
      {out.length + inc.length === 0 && <div className="muted">None recorded.</div>}

      {Object.keys(r.properties).length > 0 && (
        <>
          <h3>Properties</h3>
          <PropertyTable props={r.properties} />
        </>
      )}
      {r.tags && (
        <>
          <h3>Tags</h3>
          <PropertyTable props={r.tags} />
        </>
      )}
      <h3>Evidence</h3>
      <EvidenceList evidence={r.evidence} />
    </div>
  );
}

export function GroupPanel(props: InspectorProps & { groupId: string }) {
  const { groupId, views, projectionId, graph, index, state } = props;
  const view = views.get(projectionId)!;
  const g = view.groups.get(groupId);
  if (!g) return <div className="panel muted">Group no longer in view.</div>;
  if (g.resourceId) return <ResourcePanel {...props} id={g.resourceId} />;
  const path = groupAncestors(view.groups, g.parentId).reverse().map((x) => view.groups.get(x)!.label);
  const node = graph?.nodes.get(groupId);
  const internal = node?.internalRelationships ?? [];
  return (
    <div className="panel">
      <div className="phead">
        <span className="code">{node?.code ?? 'GRP'}</span>
        <h2>{g.label}</h2>
      </div>
      <div className="ptype">Grouping · {g.groupType} (not a resource)</div>
      <div className="path">{path.length ? path.join(' / ') : 'top level'}</div>
      <div className="muted">{g.basis}</div>
      <div className="actions">
        <button onClick={() => props.onToggleGroup(groupId)}>{state.collapsed.has(groupId) ? 'Expand' : 'Collapse'}</button>
      </div>
      {node?.kind === 'collapsed' && (
        <>
          <h3>Collapsed contents</h3>
          <div>{node.memberCount ?? 0} resources hidden inside.</div>
          {internal.length > 0 && (
            <>
              <h4>Relationships inside ({internal.length})</h4>
              <ul className="rels">
                {internal.map((rid) => {
                  const rel = index.relationship(rid)!;
                  return (
                    <li key={rid}>
                      <span className="peer" onClick={() => props.onSelectResource(rel.source)}>{index.resource(rel.source)?.name ?? rel.source}</span>
                      {' → '}
                      <span className="peer" onClick={() => props.onSelectResource(rel.target)}>{index.resource(rel.target)?.name ?? rel.target}</span>
                      <span className="muted"> {rel.type} · {disposition(rid, view, graph).text}</span>
                    </li>
                  );
                })}
              </ul>
            </>
          )}
        </>
      )}
    </div>
  );
}
