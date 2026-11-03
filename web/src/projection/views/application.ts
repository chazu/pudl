// Application architecture: application → service → member resources, with
// dependency and observed-traffic edges. Applications and services are
// logical entities, not provider resources: whatever types the vocabulary
// marks `application.group`, nested through their `application.via` edges.
// A resource is a member of the application groups it has an edge to.
// Resources that participate in dependencies without any membership go in
// an explicit "not assigned" group rather than being guessed into an
// application.

import type { Relationship, Resource } from '../../model/canonical';
import type { RelationshipCategory } from '../../model/vocabulary';
import type { ParentRef, Placement, ProjectionContext, ProjectionDefinition } from '../types';
import { touching, viaEdges } from './common';

const FLOW = new Set<RelationshipCategory>(['dependency', 'traffic']);

const isAppGroup = (r: Resource | undefined, ctx: ProjectionContext) => !!r && !!ctx.index.vocab.resourceType(r.type).application?.group;

/** Outgoing relationships to an application group: membership. */
function memberships(r: Resource, ctx: ProjectionContext): Relationship[] {
  return ctx.index.out(r.id).filter((rel) => isAppGroup(ctx.index.resource(rel.target), ctx));
}

function place(r: Resource, ctx: ProjectionContext): Placement {
  const app = ctx.index.vocab.resourceType(r.type).application;
  if (app?.group) {
    const { parents, type } = viaEdges(r, app.via, ctx);
    return { parents, basis: type ? `Part of its parent (${type}).` : 'Application (declared).' };
  }
  const member = memberships(r, ctx);
  if (member.length > 0) {
    const seen = new Set<string>();
    const parents: ParentRef[] = [];
    for (const rel of member) {
      if (seen.has(rel.target)) continue;
      seen.add(rel.target);
      parents.push({ resource: rel.target, via: rel.type });
    }
    return { parents, basis: `Member (${[...new Set(member.map((rel) => rel.type))].join(', ')}).` };
  }
  return {
    parents: [
      {
        synthetic: {
          key: 'app:unassigned',
          label: 'Not assigned to an application',
          groupType: 'unassigned',
          parent: null,
          basis: 'Participates in a dependency but has no membership relationship.',
        },
      },
    ],
    basis: 'No application membership recorded.',
  };
}

export const applicationProjection: ProjectionDefinition = {
  id: 'application',
  label: 'Application architecture',
  description: 'Logical applications and services, with dependency and data-flow edges.',
  include: (r, ctx) => isAppGroup(r, ctx) || memberships(r, ctx).length > 0 || touching(r, FLOW, ctx).length > 0,
  isGroup: (r, ctx) => isAppGroup(r, ctx),
  place,
  categories: {
    available: ['dependency', 'traffic', 'iam', 'security', 'other'],
    enabled: ['dependency', 'traffic', 'other'],
  },
  flowCategories: ['dependency', 'traffic'],
  layout: { algorithm: 'layered', direction: 'RIGHT' },
};
