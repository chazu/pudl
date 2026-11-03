// Resource inventory: administrative hierarchy, then region, then type.
// Structure over edges: relationships are available but off by default.
//
// From the vocabulary: administrative types (`admin`) are the groups; a
// provider with an owner type (`admin.owner`) nests its resources by owner
// and region; a type with `inventory.collapsed` starts collapsed.

import type { Resource } from '../../model/canonical';
import type { Placement, ProjectionContext, ProjectionDefinition } from '../types';
import { adminPlacement, ownerRef, providerRef, regionRef, typeBucketRef } from './common';

function place(r: Resource, ctx: ProjectionContext): Placement {
  const vocab = ctx.index.vocab;
  const admin = vocab.resourceType(r.type).admin;
  if (admin) {
    if (!admin.parent?.length) return { parents: [providerRef(r.provider, ctx)], basis: 'Provider.' };
    return adminPlacement(r, ctx);
  }
  if (!vocab.ownerType(r.provider)) {
    return { parents: [typeBucketRef(providerRef(r.provider, ctx), r.type, ctx)], basis: 'Provider and type.' };
  }
  const owner = ownerRef(r, ctx);
  const region = regionRef(owner, r.region ?? 'global', r.region ? 'Region attribute.' : 'No region: global or multi-region.');
  return { parents: [typeBucketRef(region, r.type, ctx)], basis: 'Owner, region and type.' };
}

export const inventoryProjection: ProjectionDefinition = {
  id: 'inventory',
  label: 'Resource inventory',
  description: 'Every resource by owner, region and type.',
  include: () => true,
  isGroup: (r, ctx) => !!ctx.index.vocab.resourceType(r.type).admin,
  place,
  categories: {
    available: ['network', 'traffic', 'dependency', 'security', 'iam', 'containment', 'other'],
    enabled: [],
  },
  flowCategories: ['network', 'traffic', 'dependency'],
  layout: { algorithm: 'packed', direction: 'DOWN' },
  defaultCollapsed: (g, vocab) => g.groupType === 'type-bucket' && !!g.bucketType && vocab.resourceType(g.bucketType).inventoryCollapsed,
};
