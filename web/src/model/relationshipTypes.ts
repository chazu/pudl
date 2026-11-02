// Relationship type registry.
//
// Every relationship type belongs to exactly one category. Categories decide
// colour, line pattern and port side, and edges are never aggregated across
// categories, so a security rule can never be drawn as, or merged into, a
// network connection. Each type also carries a `meaning`: the strongest claim
// the relationship supports, shown in the inspector next to the edge.

export type RelationshipCategory =
  /** Placement: subnet in VPC, project in folder, service in application. */
  | 'containment'
  /** Configured connectivity: routes, peering, attachments. Not reachability. */
  | 'network'
  /** Communication actually recorded (flow logs). */
  | 'traffic'
  /** Configured or declared application dependency / data flow. */
  | 'dependency'
  /** Security controls and exposure rules: security groups, firewalls, keys. */
  | 'security'
  /** Identity and access: roles, policies, bindings, trust. */
  | 'iam'
  /** Unregistered type: shown as reported, meaning unknown. */
  | 'other';

export interface CategoryInfo {
  id: RelationshipCategory;
  label: string;
  /** One-line statement of what an edge of this category does and doesn't mean. */
  meaning: string;
}

export const CATEGORIES: Record<RelationshipCategory, CategoryInfo> = {
  containment: {
    id: 'containment',
    label: 'Containment',
    meaning: 'Placement or membership. Says nothing about connectivity or access.',
  },
  network: {
    id: 'network',
    label: 'Network config',
    meaning: 'Configured connectivity. Does not establish reachability on its own.',
  },
  traffic: {
    id: 'traffic',
    label: 'Observed traffic',
    meaning: 'Communication recorded by a collector during its observation window.',
  },
  dependency: {
    id: 'dependency',
    label: 'Dependency',
    meaning: 'Configured or declared dependency. Not proof of observed communication.',
  },
  security: {
    id: 'security',
    label: 'Security control',
    meaning: 'A control or rule that permits, restricts or protects. Not a connection.',
  },
  iam: {
    id: 'iam',
    label: 'Identity & access',
    meaning: 'Policy attachment, binding or trust. Not evaluated effective permission.',
  },
  other: {
    id: 'other',
    label: 'Other',
    meaning: 'Relationship type PUDL has no registered meaning for.',
  },
};

export const CATEGORY_ORDER: RelationshipCategory[] = [
  'network',
  'traffic',
  'dependency',
  'security',
  'iam',
  'containment',
  'other',
];

export interface RelationshipTypeInfo {
  id: string;
  category: RelationshipCategory;
  /** Short edge label. */
  label: string;
  /** What the relationship does and does not establish. */
  meaning: string;
}

const T = (
  id: string,
  category: RelationshipCategory,
  label: string,
  meaning: string,
): RelationshipTypeInfo => ({ id, category, label, meaning });

const TYPES: RelationshipTypeInfo[] = [
  // containment
  T('in_organization', 'containment', 'in org', 'Administrative placement under an organization.'),
  T('in_folder', 'containment', 'in folder', 'Administrative placement under a folder.'),
  T('in_account', 'containment', 'in account', 'Owned by this account.'),
  T('in_project', 'containment', 'in project', 'Owned by this project.'),
  T('in_vpc', 'containment', 'in VPC', 'Defined inside this VPC.'),
  T('in_network', 'containment', 'in network', 'Defined inside this VPC network.'),
  T('in_subnet', 'containment', 'in subnet', 'Has an address in this subnet.'),
  T('attached_to', 'containment', 'attached to', 'Attached to this resource (interface, gateway).'),
  T('member_of', 'containment', 'member of', 'Declared member of a logical service.'),
  T('part_of', 'containment', 'part of', 'Logical service belongs to this application.'),
  // network
  T('routes_via', 'network', 'route via', 'A route table entry targets this gateway. Traffic still subject to security rules.'),
  T('associated_with', 'network', 'associated', 'Route table is associated with this subnet.'),
  T('peers_with', 'network', 'peering', 'Peering is configured. Reachability also requires routes and security rules; not evaluated.'),
  T('endpoint_for', 'network', 'endpoint for', 'Private endpoint to this service. Access still governed by IAM and endpoint policy.'),
  T('private_ip_in', 'network', 'private IP in', 'Has a private address allocated from this network.'),
  T('nat_via', 'network', 'NAT via', 'Outbound NAT is configured through this gateway.'),
  T('nat_ip', 'network', 'NAT IP', 'NAT is configured to use this address. Says nothing about which workloads egress through it.'),
  T('private_zone_for', 'network', 'private zone', 'Private DNS zone is visible from this network. Name resolution only; not connectivity.'),
  // traffic
  T('observed_flow', 'traffic', 'flow seen', 'Flow logs recorded traffic in the window. Absence of a flow is not proof of no traffic.'),
  // dependency
  T('routes_to', 'dependency', 'routes to', 'Load balancer target registration.'),
  T('health_checks', 'dependency', 'health check', 'Load balancer health-checks this target.'),
  T('reads_from', 'dependency', 'reads', 'Configured to read from this resource.'),
  T('writes_to', 'dependency', 'writes', 'Configured to write to this resource.'),
  T('publishes_to', 'dependency', 'publishes', 'Configured to publish messages to this topic or queue.'),
  T('consumes_from', 'dependency', 'consumes', 'Configured to consume from this queue or subscription.'),
  T('subscribed_to', 'dependency', 'subscribed', 'Subscription attached to this topic.'),
  T('replica_of', 'dependency', 'replica of', 'Configured as a replica of this database.'),
  T('scrapes', 'dependency', 'scrapes', 'Monitoring configuration targets this resource.'),
  // security
  T('protected_by', 'security', 'SG', 'Security group is associated. Association is not a connection.'),
  T('allows_ingress_from', 'security', 'allows from', 'A rule permits ingress from this source. Permission is not observed traffic.'),
  T('applies_to', 'security', 'applies to', 'Firewall rule targets this resource (by tag or service account).'),
  T('encrypted_by', 'security', 'encrypted by', 'Data at rest is encrypted with this key.'),
  T('exposed_to', 'security', 'exposed to', 'Configured to accept connections from this external range.'),
  // iam
  T('has_instance_profile', 'iam', 'instance role', 'Instance profile names this role.'),
  T('runs_as', 'iam', 'runs as', 'Workload identity is this service account.'),
  T('has_policy', 'iam', 'policy', 'Policy is attached. Effective permission not evaluated.'),
  T('grants_on', 'iam', 'grants on', 'Policy statement names this resource. Conditions, boundaries and SCPs not evaluated.'),
  T('trust_allows_assume', 'iam', 'trust', 'Role trust policy names this principal. The principal’s own permissions are not evaluated.'),
  T('iam_binding', 'iam', 'binding', 'IAM binding grants a role on this resource. Inherited, conditional and deny policies not evaluated.'),
];

const REGISTRY = new Map(TYPES.map((t) => [t.id, t]));

/**
 * Look up a relationship type. Unregistered types are kept, never dropped or
 * guessed into a stronger category: they land in 'other' with an explicit
 * "meaning unknown".
 */
export function relationshipType(id: string): RelationshipTypeInfo {
  return (
    REGISTRY.get(id) ?? {
      id,
      category: 'other',
      label: id,
      meaning: 'Unregistered relationship type. Meaning unknown; shown as reported.',
    }
  );
}

export function isKnownRelationshipType(id: string): boolean {
  return REGISTRY.has(id);
}

export function allRelationshipTypes(): RelationshipTypeInfo[] {
  return TYPES.slice();
}
