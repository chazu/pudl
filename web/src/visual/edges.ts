// Visual edges: resolve each candidate relationship's endpoints to visual
// nodes and ports, then aggregate.
//
// Rules that keep the picture honest:
// - Edges are never merged across categories, so a security rule cannot be
//   folded into a network edge.
// - An endpoint inside a collapsed group is lifted to the group and attached
//   to a generated boundary port. The edge still lists the real relationships.
// - Relationships with both ends inside one collapsed group are recorded on
//   the group (internalRelationships), not discarded.
// - Every candidate gets a disposition explaining whether and how it is drawn.

import type { Basis, GraphIndex, Relationship, ResourceId } from '../model/canonical';
import { CATEGORIES, relationshipType, type RelationshipCategory } from '../model/relationshipTypes';
import type { Direction } from '../layout/types';
import type { SemanticView } from '../projection/types';
import { nodeIdForResource } from './ids';
import type { EdgeDisposition, PortSide, VisualEdge, VisualNode, VisualPort } from './types';
import type { ViewState } from './viewState';
import type { Visibility } from './visibility';

interface Endpoint {
  node: string;
  /** Set for semantic ports (interfaces). */
  port?: string;
  lifted: boolean;
}

interface EdgeInput {
  view: SemanticView;
  index: GraphIndex;
  state: ViewState;
  vis: Visibility;
  nodes: Map<string, VisualNode>;
  resourceToNode: Map<ResourceId, string>;
}

export function portSides(direction: Direction): Record<'in' | 'out' | 'aux', PortSide> {
  return direction === 'RIGHT'
    ? { in: 'WEST', out: 'EAST', aux: 'SOUTH' }
    : { in: 'NORTH', out: 'SOUTH', aux: 'EAST' };
}

export function buildEdges(input: EdgeInput): {
  edges: VisualEdge[];
  dispositions: Map<string, EdgeDisposition>;
} {
  const { view, index, state, vis, nodes, resourceToNode } = input;
  const sides = portSides(state.direction);
  const flow = new Set(view.projection.flowCategories);
  const dispositions = new Map<string, EdgeDisposition>();
  const ports = new Map<string, VisualPort>();
  const byKey = new Map<string, VisualEdge>();

  const endpoint = (id: ResourceId): Endpoint | string => {
    const member = view.members.get(id);
    const gid = view.groupOf.get(id);
    if (member && !vis.members.has(id)) return vis.hiddenReason.get(id) ?? 'hidden';
    if (gid && !vis.groups.has(gid)) return vis.hiddenReason.get(id) ?? 'its group is not drawn';
    const node = resourceToNode.get(id);
    if (!node) return vis.hiddenReason.get(id) ?? 'not drawn';
    if (member?.role === 'port' && member.hostId && node === nodeIdForResource(member.hostId)) {
      const portId = `p:${id}`;
      if (!ports.has(portId)) {
        const r = index.resource(id);
        ports.set(portId, { id: portId, nodeId: node, kind: 'semantic', side: sides.aux, resourceId: id, label: r?.name });
      }
      return { node, port: portId, lifted: false };
    }
    const own = gid ?? nodeIdForResource(id);
    return { node, lifted: node !== own };
  };

  const isAncestor = (maybeAncestor: string, nodeId: string): boolean => {
    let cur = nodes.get(nodeId)?.parentId ?? null;
    while (cur) {
      if (cur === maybeAncestor) return true;
      cur = nodes.get(cur)?.parentId ?? null;
    }
    return false;
  };

  const portFor = (ep: Endpoint, category: RelationshipCategory, dir: 'in' | 'out'): string => {
    if (ep.port) return ep.port;
    const role = flow.has(category) ? dir : 'aux';
    const kind = ep.lifted ? 'boundary' : 'logical';
    const id = `${ep.node}#${kind === 'boundary' ? 'b' : 'l'}:${category}:${role}`;
    if (!ports.has(id)) ports.set(id, { id, nodeId: ep.node, kind, side: sides[role], category });
    return id;
  };

  for (const rel of view.candidates) {
    const category = relationshipType(rel.type).category;
    if (!state.enabledCategories.has(category)) {
      dispositions.set(rel.id, { kind: 'category-off' });
      continue;
    }
    const src = endpoint(rel.source);
    const tgt = endpoint(rel.target);
    if (typeof src === 'string' || typeof tgt === 'string') {
      const [rid, reason] = typeof src === 'string' ? [rel.source, src] : [rel.target, tgt as string];
      dispositions.set(rel.id, { kind: 'endpoint-hidden', resourceId: rid, reason });
      continue;
    }
    if (src.node === tgt.node && !src.port && !tgt.port) {
      dispositions.set(rel.id, { kind: 'internal', groupNodeId: src.node });
      nodes.get(src.node)?.internalRelationships?.push(rel.id);
      continue;
    }
    const enclosing = isAncestor(src.node, tgt.node) ? src.node : isAncestor(tgt.node, src.node) ? tgt.node : null;
    if (enclosing) {
      dispositions.set(rel.id, {
        kind: category === 'containment' ? 'containment' : 'to-enclosing-group',
        groupNodeId: enclosing,
      });
      continue;
    }
    const sourcePort = portFor(src, category, 'out');
    const targetPort = portFor(tgt, category, 'in');
    const aggregate = state.bundleParallel || src.lifted || tgt.lifted;
    const key = aggregate ? `${sourcePort}>${targetPort}|${category}` : rel.id;
    let edge = byKey.get(key);
    if (!edge) {
      edge = {
        id: `e:${key}`,
        source: src.node,
        sourcePort,
        target: tgt.node,
        targetPort,
        category,
        relationshipIds: [],
        types: [],
        aggregated: false,
        liftedSource: src.lifted ? src.node : undefined,
        liftedTarget: tgt.lifted ? tgt.node : undefined,
        basis: {},
        label: '',
      };
      byKey.set(key, edge);
    }
    addMember(edge, rel);
    dispositions.set(rel.id, { kind: 'drawn', edgeId: edge.id });
  }

  for (const port of ports.values()) nodes.get(port.nodeId)?.ports.push(port);
  const edges = [...byKey.values()];
  for (const e of edges) e.label = edgeLabel(e);
  countUndrawn(view, dispositions, nodes, resourceToNode);
  return { edges, dispositions };
}

function addMember(edge: VisualEdge, rel: Relationship) {
  edge.relationshipIds.push(rel.id);
  if (!edge.types.includes(rel.type)) edge.types.push(rel.type);
  edge.basis[rel.basis as Basis] = (edge.basis[rel.basis as Basis] ?? 0) + 1;
  edge.aggregated = edge.relationshipIds.length > 1;
}

function edgeLabel(e: VisualEdge): string {
  const name = e.types.length === 1 ? relationshipType(e.types[0]).label : CATEGORIES[e.category].label.toLowerCase();
  return e.aggregated ? `${e.relationshipIds.length}× ${name}` : name;
}

/** Badge count: relationships of a drawn resource hidden by the user's filters. */
function countUndrawn(
  view: SemanticView,
  dispositions: Map<string, EdgeDisposition>,
  nodes: Map<string, VisualNode>,
  resourceToNode: Map<ResourceId, string>,
) {
  for (const rel of view.candidates) {
    const d = dispositions.get(rel.id);
    if (!d || (d.kind !== 'category-off' && d.kind !== 'endpoint-hidden' && d.kind !== 'to-enclosing-group')) continue;
    for (const id of [rel.source, rel.target]) {
      const nodeId = resourceToNode.get(id);
      const node = nodeId ? nodes.get(nodeId) : undefined;
      if (node && node.resourceId === id) node.undrawnCount++;
    }
  }
}
