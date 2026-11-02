// Application architecture: application → service → member resources, with
// dependency and observed-traffic edges. Applications and services are
// logical entities declared in PUDL, not provider resources. Resources that
// participate in dependencies without any membership go in an explicit
// "not assigned" group rather than being guessed into an application.

import type { Resource } from '../../model/canonical';
import { relationshipType } from '../../model/relationshipTypes';
import type { Placement, ProjectionContext, ProjectionDefinition } from '../types';

const FLOW = new Set(['dependency', 'traffic']);

function participates(r: Resource, ctx: ProjectionContext): boolean {
  const touches = [...ctx.index.out(r.id), ...ctx.index.in(r.id)];
  return touches.some((rel) => FLOW.has(relationshipType(rel.type).category));
}

function place(r: Resource, ctx: ProjectionContext): Placement {
  if (r.type === 'logical.application') return { parents: [], basis: 'Application (declared).' };
  if (r.type === 'logical.service') return { parents: ctx.via(r, 'part_of'), basis: 'Service of application (part_of).' };
  const parents = ctx.via(r, 'member_of');
  if (parents.length > 0) return { parents, basis: 'Member of service (member_of).' };
  return {
    parents: [
      {
        synthetic: {
          key: 'app:unassigned',
          label: 'Not assigned to an application',
          groupType: 'unassigned',
          parent: null,
          basis: 'Participates in a dependency but has no member_of relationship.',
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
  include: (r, ctx) =>
    r.type === 'logical.application' ||
    r.type === 'logical.service' ||
    ctx.index.out(r.id, 'member_of').length > 0 ||
    participates(r, ctx),
  isGroup: (r) => r.type === 'logical.application' || r.type === 'logical.service',
  place,
  categories: {
    available: ['dependency', 'traffic', 'iam', 'security', 'other'],
    enabled: ['dependency', 'traffic', 'other'],
  },
  flowCategories: ['dependency', 'traffic'],
  layout: { algorithm: 'layered', direction: 'RIGHT' },
};
