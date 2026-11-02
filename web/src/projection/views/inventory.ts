// Resource inventory: administrative hierarchy, then region, then type.
// Structure over edges: relationships are available but off by default.

import type { Resource } from '../../model/canonical';
import type { Placement, ProjectionContext, ProjectionDefinition } from '../types';
import { ownerRef, providerRef, regionRef, typeBucketRef } from './common';

const ADMIN = new Set(['aws.account', 'gcp.organization', 'gcp.folder', 'gcp.project']);

function place(r: Resource, ctx: ProjectionContext): Placement {
  switch (r.type) {
    case 'aws.account':
    case 'gcp.organization':
      return { parents: [providerRef(r.provider)], basis: 'Provider.' };
    case 'gcp.folder':
    case 'gcp.project':
      return { parents: ctx.via(r, 'in_folder', 'in_organization'), basis: 'Resource hierarchy.' };
  }
  const owner = r.provider === 'aws' || r.provider === 'gcp' ? ownerRef(r, ctx) : providerRef(r.provider);
  const region = regionRef(owner, r.region ?? 'global', r.region ? 'Region attribute.' : 'No region: global or multi-region.');
  const parent = r.provider === 'aws' || r.provider === 'gcp' ? region : owner;
  return { parents: [typeBucketRef(parent, r.type)], basis: 'Owner, region and type.' };
}

export const inventoryProjection: ProjectionDefinition = {
  id: 'inventory',
  label: 'Resource inventory',
  description: 'Every resource by owner, region and type.',
  include: () => true,
  isGroup: (r) => ADMIN.has(r.type),
  place,
  categories: {
    available: ['network', 'traffic', 'dependency', 'security', 'iam', 'containment', 'other'],
    enabled: [],
  },
  flowCategories: ['network', 'traffic', 'dependency'],
  layout: { algorithm: 'packed', direction: 'DOWN' },
  defaultCollapsed: (g) => g.groupType === 'type-bucket' && g.label === 'Network interface',
};
