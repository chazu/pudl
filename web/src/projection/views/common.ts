// Placement helpers shared by the projection definitions.

import type { GraphIndex, Resource } from '../../model/canonical';
import { resourceType } from '../../model/resourceTypes';
import type { ParentRef, ProjectionContext, SyntheticGroupSpec } from '../types';

export const EXTERNAL_GROUP: SyntheticGroupSpec = {
  key: 'external',
  label: 'Outside the inventory boundary',
  groupType: 'external',
  parent: null,
  basis: 'Address ranges and principals outside any collected account. Not resources PUDL observes.',
};

/** The collected AWS account or GCP project resource named by `r.account`. */
export function ownerResource(index: GraphIndex, r: Resource): Resource | undefined {
  if (!r.account) return undefined;
  return index.byNative(r.provider === 'gcp' ? 'gcp.project' : 'aws.account', r.account);
}

export function isExternal(r: Resource): boolean {
  return r.provider === 'external';
}

/**
 * The administrative owner (AWS account or GCP project) of a resource, from
 * its `account` attribute. Falls back to a synthetic group when the owner was
 * not itself collected.
 */
export function ownerRef(r: Resource, ctx: ProjectionContext): ParentRef | null {
  if (!r.account) return null;
  const owner = ownerResource(ctx.index, r);
  if (owner) return { resource: owner.id, via: 'account attribute' };
  return {
    synthetic: {
      key: `owner:${r.provider}:${r.account}`,
      label: r.account,
      groupType: r.provider === 'gcp' ? 'gcp.project' : 'aws.account',
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

export function typeBucketRef(parent: ParentRef | null, type: string): ParentRef {
  const parentKey = !parent ? 'root' : 'resource' in parent ? parent.resource : parent.synthetic.key;
  return {
    synthetic: {
      key: `${parentKey}/type:${type}`,
      label: resourceType(type).label,
      groupType: 'type-bucket',
      parent,
      basis: `Grouped by resource type (${type}).`,
    },
  };
}

export function providerRef(provider: string): ParentRef {
  const labels: Record<string, string> = {
    aws: 'AWS',
    gcp: 'Google Cloud',
    logical: 'Logical (PUDL)',
    external: 'External',
    k8s: 'Kubernetes',
  };
  return {
    synthetic: {
      key: `provider:${provider}`,
      label: labels[provider] ?? provider,
      groupType: 'provider',
      parent: null,
      basis: 'Grouped by provider.',
    },
  };
}
