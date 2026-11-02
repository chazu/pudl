// Network topology: resources placed by network containment.
//
// AWS:  account → region → VPC → subnet → resource
// GCP:  org → folder → project → network (global) → region → subnet → resource
//
// GCP networks are global and subnets regional, so the region level sits
// inside the network there, and outside the VPC on AWS. A Shared VPC
// instance is placed in the host project's subnet; its owning project is
// shown as an `in_project` edge, not by nesting.

import type { Resource } from '../../model/canonical';
import type { Placement, ProjectionContext, ProjectionDefinition } from '../types';
import { EXTERNAL_GROUP, isExternal, ownerRef, regionRef } from './common';

const GROUP_TYPES = new Set([
  'aws.account',
  'aws.ec2.vpc',
  'aws.ec2.subnet',
  'gcp.organization',
  'gcp.folder',
  'gcp.project',
  'gcp.compute.network',
  'gcp.compute.subnetwork',
]);

const LEAF_TYPES = new Set([
  'aws.ec2.internet_gateway',
  'aws.ec2.nat_gateway',
  'aws.ec2.route_table',
  'aws.ec2.vpc_peering',
  'aws.ec2.vpc_endpoint',
  'aws.ec2.network_interface',
  'aws.ec2.instance',
  'aws.ec2.security_group',
  'aws.elbv2.load_balancer',
  'aws.rds.db_instance',
  'aws.sqs.queue',
  'gcp.compute.firewall',
  'gcp.compute.router',
  'gcp.compute.instance',
  'gcp.sql.instance',
  'external.cidr',
]);

const SUBNET_PLACED = new Set([
  'aws.ec2.instance',
  'aws.ec2.nat_gateway',
  'aws.ec2.vpc_endpoint',
  'aws.ec2.network_interface',
  'aws.elbv2.load_balancer',
  'aws.rds.db_instance',
  'gcp.compute.instance',
]);

function place(r: Resource, ctx: ProjectionContext): Placement {
  if (isExternal(r)) return { parents: [{ synthetic: EXTERNAL_GROUP }], basis: 'External address range.' };
  if (SUBNET_PLACED.has(r.type)) {
    return { parents: ctx.via(r, 'in_subnet'), basis: 'Placed by subnet (in_subnet).' };
  }
  switch (r.type) {
    case 'aws.account':
    case 'gcp.organization':
      return { parents: [], basis: 'Top-level administrative boundary.' };
    case 'aws.ec2.vpc': {
      const owner = ownerRef(r, ctx);
      return {
        parents: [regionRef(owner, r.region ?? 'unknown region', 'AWS regional services and VPCs live in a region.')],
        basis: 'VPC placed in its account and region.',
      };
    }
    case 'aws.ec2.subnet':
    case 'aws.ec2.security_group':
    case 'aws.ec2.route_table':
      return { parents: ctx.via(r, 'in_vpc'), basis: 'Defined in VPC (in_vpc).' };
    case 'aws.ec2.internet_gateway':
      return { parents: ctx.via(r, 'attached_to'), basis: 'Attached to VPC.' };
    case 'aws.ec2.vpc_peering':
      return { parents: ctx.via(r, 'peers_with'), basis: 'Placed between the peered VPCs.' };
    case 'aws.sqs.queue': {
      const owner = ownerRef(r, ctx);
      return {
        parents: [regionRef(owner, r.region ?? 'unknown region', 'AWS regional services and VPCs live in a region.')],
        basis: 'Regional service outside any VPC; reached through endpoints or public APIs.',
      };
    }
    case 'gcp.folder':
      return { parents: ctx.via(r, 'in_folder', 'in_organization'), basis: 'Resource hierarchy.' };
    case 'gcp.project':
      return { parents: ctx.via(r, 'in_folder', 'in_organization'), basis: 'Resource hierarchy.' };
    case 'gcp.compute.network':
      return { parents: ctx.via(r, 'in_project'), basis: 'Network owned by project (global).' };
    case 'gcp.compute.subnetwork':
    case 'gcp.compute.router': {
      const net = ctx.via(r, 'in_network')[0] ?? null;
      return {
        parents: [regionRef(net, r.region ?? 'unknown region', 'GCP subnets are regional within a global network.')],
        basis: 'Regional part of a global VPC network.',
      };
    }
    case 'gcp.compute.firewall':
      return { parents: ctx.via(r, 'in_network'), basis: 'Firewall rules belong to a network.' };
    case 'gcp.sql.instance': {
      const owner = ownerRef(r, ctx);
      return {
        parents: [regionRef(owner, r.region ?? 'unknown region', 'Managed service region.')],
        basis: 'Managed service in its project; network attachment shown as an edge.',
      };
    }
    default: {
      const owner = ownerRef(r, ctx);
      return { parents: owner ? [owner] : [], basis: 'Placed by owner.' };
    }
  }
}

export const networkProjection: ProjectionDefinition = {
  id: 'network',
  label: 'Network topology',
  description: 'Placement by network containment, with routes, peering, attachments and security rules.',
  include: (r) => GROUP_TYPES.has(r.type) || LEAF_TYPES.has(r.type),
  isGroup: (r) => GROUP_TYPES.has(r.type),
  place,
  ports: [{ resourceType: 'aws.ec2.network_interface', hostRelation: 'attached_to' }],
  categories: {
    available: ['network', 'traffic', 'security', 'containment', 'dependency', 'other'],
    enabled: ['network', 'traffic', 'security', 'containment', 'other'],
  },
  flowCategories: ['network', 'traffic', 'dependency'],
  layout: { algorithm: 'layered', direction: 'RIGHT' },
};
