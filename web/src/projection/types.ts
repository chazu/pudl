// Projection: canonical graph → semantic view.
//
// A projection decides which resources appear, how they are grouped, which
// relationship categories are drawn, and how the view is laid out. It works
// purely in canonical terms (resource IDs, relationship IDs); it knows nothing
// about pixels, ports-as-shapes or ELK.
//
// Grouping is a property of the view: a resource can sit in a subnet in one
// projection and in an application service in another.

import type { GraphIndex, Relationship, Resource, ResourceId } from '../model/canonical';
import type { PortRule, RelationshipCategory, Vocab } from '../model/vocabulary';
import type { LayoutConfig } from '../layout/types';

export type ProjectionId = 'network' | 'application' | 'inventory' | 'security';

/** A grouping not backed by a resource, e.g. a region or a type bucket. */
export interface SyntheticGroupSpec {
  key: string;
  label: string;
  groupType: string;
  parent: ParentRef | null;
  /** Why this grouping exists, shown in the inspector. */
  basis: string;
  /** For a type bucket: the resource type it holds. */
  bucketType?: string;
}

export type ParentRef = { resource: ResourceId; via?: string } | { synthetic: SyntheticGroupSpec };

export interface Placement {
  /** Zero parents: root. Several: placed at their lowest common ancestor. */
  parents: ParentRef[];
  basis: string;
}

export interface ProjectionContext {
  index: GraphIndex;
  /** Parents via the first relationship type in `types` that the resource has. */
  via(resource: Resource, ...types: string[]): ParentRef[];
}

export type { PortRule };

export interface ProjectionDefinition {
  id: ProjectionId;
  label: string;
  description: string;
  include(resource: Resource, ctx: ProjectionContext): boolean;
  /** Resources rendered as compound groups rather than leaf nodes. */
  isGroup(resource: Resource, ctx: ProjectionContext): boolean;
  place(resource: Resource, ctx: ProjectionContext): Placement;
  /** Resources rendered as ports on a host (from the vocabulary's portOf). */
  ports?(ctx: ProjectionContext): PortRule[];
  categories: {
    available: RelationshipCategory[];
    enabled: RelationshipCategory[];
  };
  /** Categories that read as flow (in on one side, out on the other). */
  flowCategories: RelationshipCategory[];
  layout: LayoutConfig;
  /** Groups collapsed when the view first opens. */
  defaultCollapsed?(group: SemanticGroup, vocab: Vocab): boolean;
}

export interface SemanticGroup {
  id: string;
  kind: 'resource' | 'synthetic';
  resourceId?: ResourceId;
  groupType: string;
  label: string;
  parentId: string | null;
  basis: string;
  order: number;
  /** For a type bucket: the resource type it holds. */
  bucketType?: string;
}

export interface SemanticMember {
  resourceId: ResourceId;
  parentId: string | null;
  role: 'node' | 'port' | 'unresolved';
  /** For ports: the host resource. */
  hostId?: ResourceId;
  placementBasis: string;
  placementNote?: string;
  order: number;
}

/** What the projection decided about each canonical relationship. */
export type ProjectionDisposition =
  | { kind: 'candidate' }
  | { kind: 'port-binding' }
  | { kind: 'category-not-in-view'; category: RelationshipCategory }
  | { kind: 'endpoint-not-in-view'; resourceId: ResourceId };

export interface SemanticView {
  projection: ProjectionDefinition;
  /** The vocabulary the view was projected with. */
  vocab: Vocab;
  groups: Map<string, SemanticGroup>;
  /** Leaf members, ports and unresolved placeholders, keyed by resource ID. */
  members: Map<ResourceId, SemanticMember>;
  /** Resource ID → group ID, for resources rendered as groups. */
  groupOf: Map<ResourceId, string>;
  /** Relationships that may be drawn (subject to filters). */
  candidates: Relationship[];
  dispositions: Map<string, ProjectionDisposition>;
  /** Resources the projection left out. */
  excluded: ResourceId[];
}

export const UNRESOLVED_GROUP_ID = 'g:unresolved';
