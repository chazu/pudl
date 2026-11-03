// Visual graph: what gets laid out and drawn.
//
// Every visual element points back at canonical identity (resourceId,
// relationshipIds) but owns its own structure: visual parentage, ports,
// aggregation. Canonical objects are never annotated with any of this.

import type { Basis, RelationshipId, ResourceId } from '../model/canonical';
import type { RelationshipCategory } from '../model/vocabulary';
import type { Direction } from '../layout/types';

export type PortSide = 'NORTH' | 'SOUTH' | 'EAST' | 'WEST';

export type PortKind =
  /** A real infrastructure interface (e.g. an ENI) rendered on its host. */
  | 'semantic'
  /** A category-specific attachment point (flow in/out, control). */
  | 'logical'
  /** Generated where edges terminate at a collapsed group's boundary. Not a network hop. */
  | 'boundary';

export interface VisualPort {
  id: string;
  nodeId: string;
  kind: PortKind;
  side: PortSide;
  /** Semantic ports only: the interface resource. */
  resourceId?: ResourceId;
  category?: RelationshipCategory;
  label?: string;
}

export type VisualNodeKind =
  | 'resource' // leaf canonical resource
  | 'group' // expanded compound group
  | 'collapsed' // collapsed group, drawn as a leaf
  | 'unresolved'; // referenced but not in the inventory

export interface VisualNode {
  id: string;
  kind: VisualNodeKind;
  /** The canonical resource this node stands for, if any (synthetic groups have none). */
  resourceId?: ResourceId;
  /** The semantic group this node stands for (group and collapsed kinds). */
  groupId?: string;
  parentId: string | null;
  children: string[];
  label: string;
  /** Short type code. */
  code: string;
  sublabel?: string;
  /** Resource type or group type, for styling. */
  styleType: string;
  synthetic: boolean;
  width: number;
  height: number;
  ports: VisualPort[];
  /** Model order: stable sort key for layout. */
  order: number;
  /** Collapsed groups: resources hidden inside. */
  memberCount?: number;
  /** Collapsed groups: relationships with both ends inside. */
  internalRelationships?: RelationshipId[];
  /** Relationships touching this node's resource that are not drawn in the current state. */
  undrawnCount: number;
  /** Partial graphs: relationships of this node's resource to resources not loaded yet. */
  notLoaded: number;
}

export interface VisualEdge {
  id: string;
  source: string;
  sourcePort: string;
  target: string;
  targetPort: string;
  category: RelationshipCategory;
  relationshipIds: RelationshipId[];
  types: string[];
  aggregated: boolean;
  /** Endpoints lifted to a collapsed group (the group visual ID), if any. */
  liftedSource?: string;
  liftedTarget?: string;
  basis: Partial<Record<Basis, number>>;
  label: string;
}

/** What happened to each candidate relationship in the current visual state. */
export type EdgeDisposition =
  | { kind: 'drawn'; edgeId: string }
  | { kind: 'internal'; groupNodeId: string }
  | { kind: 'containment'; groupNodeId: string }
  | { kind: 'to-enclosing-group'; groupNodeId: string }
  | { kind: 'category-off' }
  | { kind: 'endpoint-hidden'; resourceId: ResourceId; reason: string };

export interface VisualGraph {
  nodes: Map<string, VisualNode>;
  roots: string[];
  edges: VisualEdge[];
  /** Canonical resource → visual node that currently shows it (possibly a collapsed ancestor). */
  resourceToNode: Map<ResourceId, string>;
  dispositions: Map<RelationshipId, EdgeDisposition>;
  direction: Direction;
}
