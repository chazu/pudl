// Placement helpers shared by the projection definitions. Everything type-
// specific comes from the graph's vocabulary (src/model/vocabulary.ts): no
// resource or relationship type is named here.

import type { GraphIndex, Relationship, Resource } from '../../model/canonical';
import type { RelationshipCategory } from '../../model/vocabulary';
import type { ParentRef, ProjectionContext, SyntheticGroupSpec } from '../types';

export const EXTERNAL_GROUP: SyntheticGroupSpec = {
  key: 'external',
  label: 'Outside the inventory boundary',
  groupType: 'external',
  parent: null,
  basis: 'Address ranges and principals outside any collected account. Not resources PUDL observes.',
};

/**
 * The collected owner (AWS account, GCP project: its provider's
 * `admin.owner` type) named by `r.account`.
 */
export function ownerResource(index: GraphIndex, r: Resource): Resource | undefined {
  if (!r.account) return undefined;
  const type = index.vocab.ownerType(r.provider);
  return type ? index.byNative(type, r.account) : undefined;
}

/** pudl's own provider for address ranges and principals named by rules. */
export function isExternal(r: Resource): boolean {
  return r.provider === 'external';
}

/**
 * The administrative owner of a resource, from its `account` attribute.
 * Falls back to a synthetic group when the owner was not itself collected.
 */
export function ownerRef(r: Resource, ctx: ProjectionContext): ParentRef | null {
  if (!r.account) return null;
  const owner = ownerResource(ctx.index, r);
  if (owner) return { resource: owner.id, via: 'account attribute' };
  return {
    synthetic: {
      key: `owner:${r.provider}:${r.account}`,
      label: r.account,
      groupType: ctx.index.vocab.ownerType(r.provider) ?? 'owner',
      parent: null,
      basis: 'Owner named by the resource but not itself collected.',
    },
  };
}

/** A region grouping under `parent`, keyed so the same region under the same parent is one group. */
export function regionRef(parent: ParentRef | null, region: string, basis: string): ParentRef {
  const parentKey = !parent ? 'root' : 'resource' in parent ? parent.resource : parent.synthetic.key;
  return {
    synthetic: {
      key: `${parentKey}/region:${region}`,
      label: region,
      groupType: 'region',
      parent,
      basis,
    },
  };
}

export function typeBucketRef(parent: ParentRef | null, type: string, ctx: ProjectionContext): ParentRef {
  const parentKey = !parent ? 'root' : 'resource' in parent ? parent.resource : parent.synthetic.key;
  return {
    synthetic: {
      key: `${parentKey}/type:${type}`,
      label: ctx.index.vocab.resourceType(type).label,
      groupType: 'type-bucket',
      bucketType: type,
      parent,
      basis: `Grouped by resource type (${type}).`,
    },
  };
}

export function providerRef(provider: string, ctx: ProjectionContext): ParentRef {
  return {
    synthetic: {
      key: `provider:${provider}`,
      label: ctx.index.vocab.providerLabel(provider),
      groupType: 'provider',
      parent: null,
      basis: 'Grouped by provider.',
    },
  };
}

/** Parents through the first of `types` the resource has, with a basis naming it. */
export function viaEdges(r: Resource, types: string[], ctx: ProjectionContext): { parents: ParentRef[]; type?: string } {
  for (const type of types) {
    const parents = ctx.via(r, type);
    if (parents.length > 0) return { parents, type };
  }
  return { parents: [] };
}

/** Relationships touching `r` in any of `categories`. */
export function touching(r: Resource, categories: ReadonlySet<RelationshipCategory>, ctx: ProjectionContext): Relationship[] {
  const vocab = ctx.index.vocab;
  return [...ctx.index.out(r.id), ...ctx.index.in(r.id)].filter((rel) => categories.has(vocab.relationshipType(rel.type).category));
}

/**
 * Administrative placement: under the target of the first `admin.parent`
 * edge the resource has; at the root when it declares no parent edges or
 * has none of them.
 */
export function adminPlacement(r: Resource, ctx: ProjectionContext): { parents: ParentRef[]; basis: string } {
  const parent = ctx.index.vocab.resourceType(r.type).admin?.parent ?? [];
  if (parent.length === 0) return { parents: [], basis: 'Top-level administrative boundary.' };
  const { parents, type } = viaEdges(r, parent, ctx);
  return { parents, basis: type ? `Resource hierarchy (${type}).` : 'Resource hierarchy: no parent recorded.' };
}
