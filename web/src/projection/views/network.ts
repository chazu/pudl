// Network topology: resources placed by network containment.
//
// AWS:  account → region → VPC → subnet → resource
// GCP:  org → folder → project → network (global) → region → subnet → resource
//
// Which types appear and where they sit comes from the vocabulary: each
// type's `network` role (group, via edges, region) and `admin` role
// (administrative hierarchy). A Shared VPC instance is placed in the host
// project's subnet by its `in_subnet`-style via edge; its owning project is
// shown as an edge, not by nesting.

import type { Resource } from '../../model/canonical';
import type { RelationshipCategory } from '../../model/vocabulary';
import type { ParentRef, Placement, ProjectionContext, ProjectionDefinition } from '../types';
import { adminPlacement, EXTERNAL_GROUP, isExternal, ownerRef, regionRef, touching, viaEdges } from './common';

const AVAILABLE: RelationshipCategory[] = ['network', 'traffic', 'security', 'containment', 'dependency', 'other'];
const AVAILABLE_SET = new Set(AVAILABLE);

function include(r: Resource, ctx: ProjectionContext): boolean {
  const t = ctx.index.vocab.resourceType(r.type);
  if (t.admin || t.network) return true;
  // External ranges and principals: shown when a relationship this view draws names them.
  return isExternal(r) && touching(r, AVAILABLE_SET, ctx).length > 0;
}

function place(r: Resource, ctx: ProjectionContext): Placement {
  if (isExternal(r)) return { parents: [{ synthetic: EXTERNAL_GROUP }], basis: 'External address range.' };
  const t = ctx.index.vocab.resourceType(r.type);
  if (t.admin) return adminPlacement(r, ctx);

  const role = t.network ?? { via: [], region: false };
  const via = viaEdges(r, role.via ?? [], ctx);
  let parents: ParentRef[] = via.parents;
  let basis = via.type ? `Placed by ${via.type}.` : 'Placed by owner.';
  if (parents.length === 0) {
    const owner = ownerRef(r, ctx);
    parents = owner ? [owner] : [];
    if (role.via?.length) basis = `No ${role.via.join(' or ')} recorded; placed by owner.`;
  }
  if (role.region && r.region) {
    const region = r.region;
    parents = (parents.length ? parents : [null]).map((p) => regionRef(p, region, 'Regional resource.'));
    basis += ` In region ${region}.`;
  }
  return { parents, basis };
}

export const networkProjection: ProjectionDefinition = {
  id: 'network',
  label: 'Network topology',
  description: 'Placement by network containment, with routes, peering, attachments and security rules.',
  include,
  isGroup: (r, ctx) => {
    const t = ctx.index.vocab.resourceType(r.type);
    return !!t.admin || !!t.network?.group;
  },
  place,
  ports: (ctx) => ctx.index.vocab.portRules(),
  categories: {
    available: AVAILABLE,
    enabled: ['network', 'traffic', 'security', 'containment', 'other'],
  },
  flowCategories: ['network', 'traffic', 'dependency'],
  layout: { algorithm: 'layered', direction: 'RIGHT' },
};
