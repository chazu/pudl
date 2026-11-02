// Semantic view + view state → visual graph (nodes and groups).
// Edge construction lives in edges.ts.

import type { GraphIndex, ResourceId } from '../model/canonical';
import { resourceType } from '../model/resourceTypes';
import { groupAncestors } from '../projection/project';
import type { SemanticGroup, SemanticView } from '../projection/types';
import { buildEdges } from './edges';
import { SIZES, nodeIdForResource } from './ids';
import type { VisualGraph, VisualNode } from './types';
import type { ViewState } from './viewState';
import { computeVisibility } from './visibility';

export function buildVisualGraph(view: SemanticView, index: GraphIndex, state: ViewState): VisualGraph {
  const vis = computeVisibility(view, index, state);
  const nodes = new Map<string, VisualNode>();
  const resourceToNode = new Map<ResourceId, string>();

  /** Topmost collapsed drawn group enclosing `parentId` (inclusive), if any. */
  const collapsedAncestor = (parentId: string | null): string | null => {
    let top: string | null = null;
    for (const gid of groupAncestors(view.groups, parentId)) {
      if (vis.groups.has(gid) && state.collapsed.has(gid)) top = gid;
    }
    return top;
  };

  const groups = [...view.groups.values()]
    .filter((g) => vis.groups.has(g.id))
    .sort((a, b) => a.order - b.order);

  for (const g of groups) {
    const parent = vis.drawnParent(g.parentId);
    const rep = collapsedAncestor(parent);
    if (rep) {
      if (g.resourceId) resourceToNode.set(g.resourceId, rep);
      continue;
    }
    const collapsed = state.collapsed.has(g.id);
    nodes.set(g.id, groupNode(g, parent, collapsed, index));
    if (g.resourceId) resourceToNode.set(g.resourceId, g.id);
  }

  const members = [...view.members.values()]
    .filter((m) => vis.members.has(m.resourceId) && m.role !== 'port')
    .sort((a, b) => a.order - b.order);
  for (const m of members) {
    const parent = vis.drawnParent(m.parentId);
    const rep = collapsedAncestor(parent);
    if (rep) {
      resourceToNode.set(m.resourceId, rep);
      const node = nodes.get(rep)!;
      node.memberCount = (node.memberCount ?? 0) + 1;
      continue;
    }
    const id = nodeIdForResource(m.resourceId);
    const r = index.resource(m.resourceId);
    const info = resourceType(r?.type ?? 'unresolved');
    const unresolved = m.role === 'unresolved';
    const size = unresolved ? SIZES.unresolved : SIZES.leaf;
    nodes.set(id, {
      id,
      kind: unresolved ? 'unresolved' : 'resource',
      resourceId: m.resourceId,
      parentId: parent,
      children: [],
      label: r?.name ?? shortRef(m.resourceId),
      code: unresolved ? '?' : info.code,
      sublabel: unresolved ? 'not in inventory' : sublabelFor(r?.nativeId, r?.properties),
      styleType: unresolved ? 'unresolved' : info.cls,
      synthetic: false,
      ...size,
      ports: [],
      order: m.order,
      undrawnCount: 0,
    });
    resourceToNode.set(m.resourceId, id);
  }
  // Ports ride on their host's node (or its collapsed representative).
  for (const m of view.members.values()) {
    if (m.role === 'port' && m.hostId && vis.members.has(m.resourceId)) {
      const host = resourceToNode.get(m.hostId);
      if (host) resourceToNode.set(m.resourceId, host);
    }
  }

  for (const node of nodes.values()) {
    if (node.parentId) nodes.get(node.parentId)?.children.push(node.id);
  }
  const roots = [...nodes.values()].filter((n) => !n.parentId).map((n) => n.id);

  const { edges, dispositions } = buildEdges({ view, index, state, vis, nodes, resourceToNode });
  return { nodes, roots, edges, resourceToNode, dispositions, direction: state.direction };
}

function groupNode(g: SemanticGroup, parentId: string | null, collapsed: boolean, index: GraphIndex): VisualNode {
  const r = g.resourceId ? index.resource(g.resourceId) : undefined;
  const code = r ? resourceType(r.type).code : groupCode(g.groupType);
  const size = collapsed ? SIZES.collapsed : { width: 0, height: 0 };
  return {
    id: g.id,
    kind: collapsed ? 'collapsed' : 'group',
    resourceId: g.resourceId,
    groupId: g.id,
    parentId,
    children: [],
    label: g.label,
    code,
    sublabel: r ? sublabelFor(r.nativeId, r.properties) : undefined,
    styleType: r ? resourceType(r.type).cls : g.groupType,
    synthetic: g.kind === 'synthetic',
    ...size,
    ports: [],
    order: g.order,
    memberCount: collapsed ? 0 : undefined,
    internalRelationships: collapsed ? [] : undefined,
    undrawnCount: 0,
  };
}

function groupCode(groupType: string): string {
  const codes: Record<string, string> = {
    region: 'RGN',
    'type-bucket': 'TYPE',
    provider: 'PRV',
    external: 'EXT',
    unresolved: '?',
    unassigned: 'NONE',
  };
  return codes[groupType] ?? groupType.slice(0, 4).toUpperCase();
}

function sublabelFor(nativeId: string | undefined, props: Record<string, unknown> | undefined): string | undefined {
  const cidr = props?.CidrBlock ?? props?.ipCidrRange;
  if (typeof cidr === 'string') return cidr;
  if (!nativeId) return undefined;
  return nativeId.length > 30 ? `…${nativeId.slice(-29)}` : nativeId;
}

function shortRef(id: string): string {
  const tail = id.split(':').pop() ?? id;
  return tail.length > 28 ? `…${tail.slice(-27)}` : tail;
}
