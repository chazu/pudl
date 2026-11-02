// The projection engine: applies a ProjectionDefinition to a canonical graph.
//
// It never mutates the graph. Every canonical relationship gets a
// disposition, so nothing is dropped without a recorded reason.

import type { GraphIndex, Resource, ResourceId } from '../model/canonical';
import { relationshipType } from '../model/relationshipTypes';
import {
  UNRESOLVED_GROUP_ID,
  type ParentRef,
  type Placement,
  type ProjectionContext,
  type ProjectionDefinition,
  type ProjectionDisposition,
  type SemanticGroup,
  type SemanticMember,
  type SemanticView,
  type SyntheticGroupSpec,
} from './types';

export function makeContext(index: GraphIndex): ProjectionContext {
  return {
    index,
    via(resource: Resource, ...types: string[]): ParentRef[] {
      for (const type of types) {
        const targets = index.out(resource.id, type).filter((rel) => index.has(rel.target));
        if (targets.length > 0) return targets.map((rel) => ({ resource: rel.target, via: type }));
      }
      return [];
    },
  };
}

export function project(index: GraphIndex, def: ProjectionDefinition): SemanticView {
  const ctx = makeContext(index);
  const included = new Set<ResourceId>();
  const excluded: ResourceId[] = [];
  for (const r of index.graph.resources) {
    if (def.include(r, ctx)) included.add(r.id);
    else excluded.push(r.id);
  }

  const groups = new Map<string, SemanticGroup>();
  const groupOf = new Map<ResourceId, string>();
  const members = new Map<ResourceId, SemanticMember>();
  let order = 0;

  // Ports: resources rendered on a host node rather than as nodes.
  const portHost = new Map<ResourceId, { host: ResourceId; relation: string }>();
  for (const rule of def.ports ?? []) {
    for (const r of index.graph.resources) {
      if (r.type !== rule.resourceType || !included.has(r.id)) continue;
      const host = index
        .targets(r.id, rule.hostRelation)
        .find((h) => included.has(h.id) && !def.isGroup(h));
      if (host) portHost.set(r.id, { host: host.id, relation: rule.hostRelation });
    }
  }

  const resolving = new Set<ResourceId>();

  /** Group ID for a resource rendered as a group; undefined if it is not one in this view. */
  function groupForResource(id: ResourceId): string | undefined {
    const known = groupOf.get(id);
    if (known) return known;
    const r = index.resource(id);
    if (!r || !included.has(id) || !def.isGroup(r) || resolving.has(id)) return undefined;
    resolving.add(id);
    const placement = def.place(r, ctx);
    const { parentId } = resolveParent(placement);
    const gid = `g:${id}`;
    groups.set(gid, {
      id: gid,
      kind: 'resource',
      resourceId: id,
      groupType: r.type,
      label: r.name,
      parentId,
      basis: placement.basis,
      order: order++,
    });
    groupOf.set(id, gid);
    resolving.delete(id);
    return gid;
  }

  function syntheticGroup(spec: SyntheticGroupSpec): string {
    const gid = `s:${spec.key}`;
    if (!groups.has(gid)) {
      const parentId = spec.parent ? (refToGroup(spec.parent) ?? null) : null;
      groups.set(gid, {
        id: gid,
        kind: 'synthetic',
        groupType: spec.groupType,
        label: spec.label,
        parentId,
        basis: spec.basis,
        order: order++,
      });
    }
    return gid;
  }

  function refToGroup(ref: ParentRef): string | undefined {
    return 'resource' in ref ? groupForResource(ref.resource) : syntheticGroup(ref.synthetic);
  }

  function resolveParent(p: Placement): { parentId: string | null; note?: string } {
    const ids = p.parents.map(refToGroup).filter((g): g is string => g !== undefined);
    if (ids.length === 0) {
      return {
        parentId: null,
        note: p.parents.length > 0 ? 'Its placement parent is not in this view; shown at top level.' : undefined,
      };
    }
    const unique = [...new Set(ids)];
    if (unique.length === 1) return { parentId: unique[0] };
    const lca = lowestCommonAncestor(groups, unique);
    const names = unique.map((g) => groups.get(g)?.label ?? g).join(', ');
    const where = lca ? `their common ancestor ${groups.get(lca)?.label}` : 'top level';
    return {
      parentId: lca,
      note: `Has ${unique.length} placement parents (${names}); shown at ${where} rather than picking one.`,
    };
  }

  for (const r of index.graph.resources) {
    if (!included.has(r.id)) continue;
    if (def.isGroup(r)) {
      groupForResource(r.id);
      continue;
    }
    const port = portHost.get(r.id);
    if (port) {
      members.set(r.id, {
        resourceId: r.id,
        parentId: null,
        role: 'port',
        hostId: port.host,
        placementBasis: `Shown as an interface of its host (${port.relation}).`,
        order: order++,
      });
      continue;
    }
    const placement = def.place(r, ctx);
    const { parentId, note } = resolveParent(placement);
    members.set(r.id, {
      resourceId: r.id,
      parentId,
      role: 'node',
      placementBasis: placement.basis,
      placementNote: note,
      order: order++,
    });
  }
  for (const m of members.values()) {
    if (m.role === 'port' && m.hostId) m.parentId = members.get(m.hostId)?.parentId ?? null;
  }

  // Relationships.
  const dispositions = new Map<string, ProjectionDisposition>();
  const candidates = [];
  const inView = (id: ResourceId) => members.has(id) || groupOf.has(id);
  for (const rel of index.graph.relationships) {
    const port = portHost.get(rel.source);
    if (port && port.host === rel.target && port.relation === rel.type) {
      dispositions.set(rel.id, { kind: 'port-binding' });
      continue;
    }
    const category = relationshipType(rel.type).category;
    if (!def.categories.available.includes(category)) {
      dispositions.set(rel.id, { kind: 'category-not-in-view', category });
      continue;
    }
    const srcMissing = !index.has(rel.source);
    const tgtMissing = !index.has(rel.target);
    const notInView = [rel.source, rel.target].find((id) => index.has(id) && !inView(id));
    if (notInView || (srcMissing && tgtMissing)) {
      dispositions.set(rel.id, { kind: 'endpoint-not-in-view', resourceId: notInView ?? rel.source });
      continue;
    }
    for (const missing of [srcMissing ? rel.source : null, tgtMissing ? rel.target : null]) {
      if (missing && !members.has(missing)) {
        if (!groups.has(UNRESOLVED_GROUP_ID)) {
          groups.set(UNRESOLVED_GROUP_ID, {
            id: UNRESOLVED_GROUP_ID,
            kind: 'synthetic',
            groupType: 'unresolved',
            label: 'Not in inventory',
            parentId: null,
            basis: 'Referenced by a relationship but never collected. PUDL knows only the reference.',
            order: order++,
          });
        }
        members.set(missing, {
          resourceId: missing,
          parentId: UNRESOLVED_GROUP_ID,
          role: 'unresolved',
          placementBasis: 'Referenced but not in the inventory.',
          order: order++,
        });
      }
    }
    dispositions.set(rel.id, { kind: 'candidate' });
    candidates.push(rel);
  }

  return { projection: def, groups, members, groupOf, candidates, dispositions, excluded };
}

/** Ancestor chain of a group, nearest first, including itself. */
export function groupAncestors(groups: Map<string, SemanticGroup>, gid: string | null): string[] {
  const chain: string[] = [];
  const seen = new Set<string>();
  let cur = gid;
  while (cur && !seen.has(cur)) {
    seen.add(cur);
    chain.push(cur);
    cur = groups.get(cur)?.parentId ?? null;
  }
  return chain;
}

export function lowestCommonAncestor(groups: Map<string, SemanticGroup>, ids: string[]): string | null {
  const chains = ids.map((id) => groupAncestors(groups, id));
  const [first, ...rest] = chains;
  for (const candidate of first) {
    if (rest.every((c) => c.includes(candidate))) return candidate;
  }
  return null;
}
