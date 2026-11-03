// Security relationships: principals, roles, policies, bindings, trust,
// security groups, firewall rules and keys, plus the resources they name.
// Grouped only by administrative owner; no network nesting, so nothing here
// reads as a network path.
//
// From the vocabulary: owner types (`admin.owner`) are the groups; types with
// `security.control` always appear; anything else appears when it touches a
// security or IAM relationship.

import type { Resource } from '../../model/canonical';
import type { RelationshipCategory } from '../../model/vocabulary';
import type { Placement, ProjectionContext, ProjectionDefinition } from '../types';
import { EXTERNAL_GROUP, isExternal, ownerRef, touching } from './common';

const SECURITY = new Set<RelationshipCategory>(['security', 'iam']);

const touchesSecurity = (r: Resource, ctx: ProjectionContext) => touching(r, SECURITY, ctx).length > 0;

/** Hosts of ports (e.g. interfaces) that carry security relationships, so the port can be drawn on them. */
function hostsSecurePort(r: Resource, ctx: ProjectionContext): boolean {
  return ctx.index.in(r.id).some((rel) => {
    const port = ctx.index.resource(rel.source);
    return !!port && ctx.index.vocab.resourceType(port.type).portOf === rel.type && touchesSecurity(port, ctx);
  });
}

function place(r: Resource, ctx: ProjectionContext): Placement {
  if (isExternal(r)) return { parents: [{ synthetic: EXTERNAL_GROUP }], basis: 'External principal or range.' };
  if (ctx.index.vocab.resourceType(r.type).admin?.owner) return { parents: [], basis: 'Administrative boundary.' };
  const owner = ownerRef(r, ctx);
  return { parents: owner ? [owner] : [], basis: 'Placed by owning account or project.' };
}

export const securityProjection: ProjectionDefinition = {
  id: 'security',
  label: 'Security relationships',
  description: 'IAM principals, policies, trust, security controls and exposure.',
  include: (r, ctx) => {
    const t = ctx.index.vocab.resourceType(r.type);
    return (
      !!t.admin?.owner ||
      t.securityControl ||
      (t.cls !== 'logical' && touchesSecurity(r, ctx)) ||
      hostsSecurePort(r, ctx)
    );
  },
  isGroup: (r, ctx) => !!ctx.index.vocab.resourceType(r.type).admin?.owner,
  place,
  ports: (ctx) => ctx.index.vocab.portRules(),
  categories: {
    available: ['iam', 'security', 'network', 'other'],
    enabled: ['iam', 'security', 'other'],
  },
  flowCategories: ['iam', 'security'],
  layout: { algorithm: 'layered', direction: 'RIGHT' },
};
