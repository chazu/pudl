// The graph vocabulary: what resource types and relationship types mean.
//
// None of it is written here. pudl declares it in CUE (schemas' `_pudl.graph`
// blocks and `graph_*` datalog facts) and sends it with every graph as
// `vocabulary` (docs/design/2026-11-02-graph-vocabulary-in-cue.md). This
// module only indexes it and supplies safe fallbacks: an undeclared resource
// type renders with a code from its last segment and class 'external'; an
// undeclared relationship type lands in 'other' with "meaning unknown",
// never guessed into a stronger category.
//
// What stays in the UI is styling: categories and classes are fixed sets the
// stylesheet colours (src/render/edgeStyle.ts, --cat-* and --cls-* in
// styles.css). Their names and meanings come from the vocabulary.

/** Relationship categories the UI can style. Categories decide colour, line pattern and port side. */
export const CATEGORY_IDS = ['containment', 'network', 'traffic', 'dependency', 'security', 'iam', 'other'] as const;
export type RelationshipCategory = (typeof CATEGORY_IDS)[number];

/** Resource classes the UI can style. */
export const RESOURCE_CLASSES = [
  'admin',
  'network',
  'compute',
  'data',
  'messaging',
  'edge',
  'security',
  'identity',
  'logical',
  'external',
] as const;
export type ResourceClass = (typeof RESOURCE_CLASSES)[number];

// ---- Wire format (the `vocabulary` field of a graph document) ----

export interface CategoryEntry {
  id: string;
  label: string;
  meaning: string;
  order: number;
  /** Where it was declared: fact rule name and file position. */
  source?: string;
}

export interface EdgeTypeEntry {
  type: string;
  category: string;
  label: string;
  meaning: string;
  /** Where it was declared: fact rule name and file position. */
  source?: string;
}

export interface ProviderEntry {
  provider: string;
  label: string;
  source?: string;
}

export interface AdminRole {
  /** Other resources' `account` attribute names a resource of this type. */
  owner?: boolean;
  /** Edges to the administrative parent; absent or empty: top of the hierarchy. */
  parent?: string[];
}

export interface NetworkRole {
  group?: boolean;
  /** Placed under the target of the first of these edges it has, else under its owner. */
  via?: string[];
  /** Wrap the parent in a region group when the resource has a region. */
  region?: boolean;
}

export interface ResourceTypeEntry {
  type: string;
  code: string;
  label: string;
  /** Empty when a schema declares roles without display info: rendered as an unknown type's fallback. */
  class: string;
  /** The schema that declared it; absent for graph_resource_type facts. */
  schema?: string;
  /** Where a graph_resource_type fact declared it. */
  source?: string;
  admin?: AdminRole | null;
  network?: NetworkRole | null;
  /** Drawn as a port on the resource this edge points to. */
  portOf?: string;
  security?: { control?: boolean } | null;
  inventory?: { collapsed?: boolean } | null;
  application?: { group?: boolean; via?: string[] } | null;
}

export interface Vocabulary {
  categories: CategoryEntry[];
  edgeTypes: EdgeTypeEntry[];
  providers: ProviderEntry[];
  resourceTypes: ResourceTypeEntry[];
  problems?: string[];
}

export const EMPTY_VOCABULARY: Vocabulary = { categories: [], edgeTypes: [], providers: [], resourceTypes: [] };

// ---- Lookups ----

export interface CategoryInfo {
  id: RelationshipCategory;
  label: string;
  /** One-line statement of what an edge of this category does and doesn't mean. */
  meaning: string;
}

export interface RelationshipTypeInfo {
  id: string;
  category: RelationshipCategory;
  /** Short edge label. */
  label: string;
  /** What the relationship does and does not establish. */
  meaning: string;
}

export interface ResourceTypeInfo {
  type: string;
  /** 2–5 character code shown on the node. */
  code: string;
  label: string;
  cls: ResourceClass;
  /** False when the vocabulary does not declare the type. */
  known: boolean;
  admin: AdminRole | null;
  network: NetworkRole | null;
  portOf: string;
  securityControl: boolean;
  inventoryCollapsed: boolean;
  application: { group: boolean; via: string[] } | null;
}

/** A resource rendered as a port on a host instead of as a node. */
export interface PortRule {
  resourceType: string;
  /** Relationship from the port resource to its host. */
  hostRelation: string;
}

const isCategory = (s: string): s is RelationshipCategory => (CATEGORY_IDS as readonly string[]).includes(s);
const isClass = (s: string): s is ResourceClass => (RESOURCE_CLASSES as readonly string[]).includes(s);

/** The provider of a resource type: its first segment. */
export function providerOfType(type: string): string {
  return type.split('.', 1)[0];
}

/** Indexed, read-only view of a vocabulary. */
export class Vocab {
  readonly source: Vocabulary;
  private readonly categories = new Map<string, CategoryEntry>();
  private readonly edges = new Map<string, RelationshipTypeInfo>();
  private readonly types = new Map<string, ResourceTypeInfo>();
  private readonly providers = new Map<string, string>();
  private readonly order: RelationshipCategory[];

  constructor(v: Vocabulary | undefined | null) {
    const vocab = v ?? EMPTY_VOCABULARY;
    this.source = vocab;
    for (const c of vocab.categories ?? []) this.categories.set(c.id, c);
    for (const e of vocab.edgeTypes ?? []) {
      this.edges.set(e.type, {
        id: e.type,
        category: isCategory(e.category) ? e.category : 'other',
        label: e.label || e.type,
        meaning: e.meaning,
      });
    }
    for (const p of vocab.providers ?? []) this.providers.set(p.provider, p.label);
    for (const t of vocab.resourceTypes ?? []) this.types.set(t.type, toInfo(t));
    // Declared categories in their declared order, then any the vocabulary
    // left out, so every styled category stays reachable.
    const declared = [...(vocab.categories ?? [])]
      .filter((c) => isCategory(c.id))
      .sort((a, b) => a.order - b.order || a.id.localeCompare(b.id))
      .map((c) => c.id as RelationshipCategory);
    this.order = [...declared, ...CATEGORY_IDS.filter((c) => !declared.includes(c))];
  }

  get problems(): string[] {
    return this.source.problems ?? [];
  }

  resourceType(type: string): ResourceTypeInfo {
    return this.types.get(type) ?? unknownType(type);
  }

  /** Declared resource types. */
  resourceTypes(): ResourceTypeInfo[] {
    return [...this.types.values()];
  }

  /**
   * Look up a relationship type. Unregistered types are kept, never dropped or
   * guessed into a stronger category: they land in 'other' with an explicit
   * "meaning unknown".
   */
  relationshipType(id: string): RelationshipTypeInfo {
    return (
      this.edges.get(id) ?? {
        id,
        category: 'other',
        label: id,
        meaning: 'Unregistered relationship type. Meaning unknown; shown as reported.',
      }
    );
  }

  isKnownRelationshipType(id: string): boolean {
    return this.edges.has(id);
  }

  relationshipTypes(): RelationshipTypeInfo[] {
    return [...this.edges.values()];
  }

  category(id: RelationshipCategory): CategoryInfo {
    const c = this.categories.get(id);
    if (c) return { id, label: c.label, meaning: c.meaning };
    return { id, label: id.charAt(0).toUpperCase() + id.slice(1), meaning: 'No meaning declared for this category.' };
  }

  /** Every styled category, declared order first. */
  categoryOrder(): RelationshipCategory[] {
    return this.order;
  }

  /** Containment types: followed when loading a neighbourhood, so what is loaded can still be placed. */
  containmentTypes(): string[] {
    return this.relationshipTypes()
      .filter((t) => t.category === 'containment')
      .map((t) => t.id);
  }

  providerLabel(provider: string): string {
    return this.providers.get(provider) ?? provider;
  }

  /** The type whose resources a provider's `account` attributes name (AWS account, GCP project). */
  ownerType(provider: string): string | undefined {
    return this.resourceTypes()
      .filter((t) => t.admin?.owner && providerOfType(t.type) === provider)
      .map((t) => t.type)
      .sort()[0];
  }

  portRules(): PortRule[] {
    return this.resourceTypes()
      .filter((t) => t.portOf)
      .map((t) => ({ resourceType: t.type, hostRelation: t.portOf }));
  }
}

function toInfo(t: ResourceTypeEntry): ResourceTypeInfo {
  return {
    type: t.type,
    code: t.code || fallbackCode(t.type),
    label: t.label || t.type,
    cls: isClass(t.class) ? t.class : 'external',
    known: !!(t.code || t.label || t.class),
    admin: t.admin ? { owner: !!t.admin.owner, parent: t.admin.parent ?? [] } : null,
    network: t.network ? { group: !!t.network.group, via: t.network.via ?? [], region: !!t.network.region } : null,
    portOf: t.portOf ?? '',
    securityControl: !!t.security?.control,
    inventoryCollapsed: !!t.inventory?.collapsed,
    application: t.application ? { group: !!t.application.group, via: t.application.via ?? [] } : null,
  };
}

function fallbackCode(type: string): string {
  return type.split('.').pop()?.slice(0, 4).toUpperCase() || '?';
}

function unknownType(type: string): ResourceTypeInfo {
  return {
    type,
    code: fallbackCode(type),
    label: type,
    cls: 'external',
    known: false,
    admin: null,
    network: null,
    portOf: '',
    securityControl: false,
    inventoryCollapsed: false,
    application: null,
  };
}
