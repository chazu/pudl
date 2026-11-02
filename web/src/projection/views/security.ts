// Security relationships: principals, roles, policies, bindings, trust,
// security groups, firewall rules and keys, plus the resources they name.
// Grouped only by administrative owner; no network nesting, so nothing here
// reads as a network path.

import type { Resource } from '../../model/canonical';
import { relationshipType } from '../../model/relationshipTypes';
import type { Placement, ProjectionContext, ProjectionDefinition } from '../types';
import { EXTERNAL_GROUP, isExternal, ownerRef } from './common';

const CONTROL_TYPES = new Set([
  'aws.iam.user',
  'aws.iam.role',
  'aws.iam.policy',
  'aws.ec2.security_group',
  'aws.kms.key',
  'gcp.iam.service_account',
  'gcp.compute.firewall',
]);

const SECURITY = new Set(['security', 'iam']);

function touchesSecurity(r: Resource, ctx: ProjectionContext): boolean {
  const touches = [...ctx.index.out(r.id), ...ctx.index.in(r.id)];
  return touches.some((rel) => SECURITY.has(relationshipType(rel.type).category));
}

function place(r: Resource, ctx: ProjectionContext): Placement {
  if (isExternal(r)) return { parents: [{ synthetic: EXTERNAL_GROUP }], basis: 'External principal or range.' };
  if (r.type === 'aws.account' || r.type === 'gcp.project') return { parents: [], basis: 'Administrative boundary.' };
  const owner = ownerRef(r, ctx);
  return { parents: owner ? [owner] : [], basis: 'Placed by owning account or project.' };
}

export const securityProjection: ProjectionDefinition = {
  id: 'security',
  label: 'Security relationships',
  description: 'IAM principals, policies, trust, security controls and exposure.',
  include: (r, ctx) =>
    r.type === 'aws.account' ||
    r.type === 'gcp.project' ||
    CONTROL_TYPES.has(r.type) ||
    (r.provider !== 'logical' && touchesSecurity(r, ctx)) ||
    // Hosts of interfaces that carry security groups, so the interface can be a port.
    ctx.index.sources(r.id, 'attached_to').some((iface) => touchesSecurity(iface, ctx)),
  isGroup: (r) => r.type === 'aws.account' || r.type === 'gcp.project',
  place,
  ports: [{ resourceType: 'aws.ec2.network_interface', hostRelation: 'attached_to' }],
  categories: {
    available: ['iam', 'security', 'network', 'other'],
    enabled: ['iam', 'security', 'other'],
  },
  flowCategories: ['iam', 'security'],
  layout: { algorithm: 'layered', direction: 'RIGHT' },
};
