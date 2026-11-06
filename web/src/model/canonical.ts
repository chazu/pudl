// Canonical resource graph.
//
// These types describe what PUDL knows about infrastructure, independent of
// any view. Nothing here carries screen positions, visual parents, ports or
// routing: those belong to the visual model (src/visual) and the layout
// result (src/layout). Projections read this graph; they never mutate it.

import { Vocab, type Vocabulary } from './vocabulary';

export type ResourceId = string;
export type RelationshipId = string;

/** Where a fact about the graph came from. Mirrors PUDL's evidence vocabulary. */
export type Basis =
  /** Seen in a collector's output (an observation snapshot). */
  | 'observed'
  /** Stated by configuration or a manifest, not seen at runtime. */
  | 'declared'
  /** Derived by a rule or heuristic (tags, naming); weakest. */
  | 'inferred'
  /** No provenance available. Never upgraded to anything stronger. */
  | 'unknown';

export interface Evidence {
  basis: Basis;
  /** Collector, file or rule that produced this, e.g. "aws ec2 describe-instances". */
  source: string;
  /** ISO-8601 time of observation, if known. */
  observedAt?: string;
  /** PUDL snapshot or catalog entry the evidence points at, if any. */
  snapshotId?: string;
  /** Free-form note: which field, which rule. */
  detail?: string;
}

/** First segment of the resource type ('aws', 'gcp', 'k8s', ...), or 'external' / 'logical'. */
export type Provider = string;

export interface Resource {
  /** Stable identity. Views and selection key on this. */
  id: ResourceId;
  /** PUDL resource_type, e.g. "aws.ec2.instance". */
  type: string;
  name: string;
  provider: Provider;
  /** Provider-native identifier (ARN, selfLink, instance ID). */
  nativeId?: string;
  /** AWS account ID or GCP project ID that owns the resource. */
  account?: string;
  region?: string;
  zone?: string;
  properties: Record<string, unknown>;
  tags?: Record<string, string>;
  evidence?: Evidence[];
  /** pudl schema and catalog entry the resource was read from (live exports). */
  schema?: string;
  entryId?: string;
  /**
   * Every cataloged record of a resource that several schemas describe (pudl's
   * same_resource), e.g. a cluster seen by GCP and by kubectl. Absent when the
   * resource has one record; the fields above then come from the first.
   */
  representations?: Representation[];
}

/** One cataloged record of a resource that several schemas describe. */
export interface Representation {
  /** The record's own resource ID; the resource's id is its group's. */
  id: ResourceId;
  schema: string;
  type: string;
  entryId: string;
  origin: string;
}

export interface Relationship {
  id: RelationshipId;
  /** Relationship type, declared in the graph's vocabulary (src/model/vocabulary.ts). */
  type: string;
  source: ResourceId;
  target: ResourceId;
  basis: Basis;
  properties?: Record<string, unknown>;
  evidence?: Evidence[];
}

export type GraphOrigin = 'live' | 'mock' | 'file';

export interface HiddenSchema {
  schema: string;
  entries: number;
  /** 'no-identity': the records resolve to no resource identity; 'internal': pudl's own records. */
  reason: 'no-identity' | 'internal' | string;
}

export interface Coverage {
  entries: number;
  resources: number;
  hidden?: HiddenSchema[];
  edgeRules: number;
}

export interface GraphMeta {
  origin: GraphOrigin;
  /** Human label for the data source, shown in the UI. */
  label: string;
  generatedAt?: string;
  /** Problems the producer reported (unreadable payloads, ambiguous references). */
  warnings?: string[];
  /** Producer's change stamp for this graph (pudl ui); compared to the source's current version. */
  version?: string;
  /** Set when the graph is the catalog as recorded at a past time (RFC 3339). */
  asOf?: string;
  /**
   * A partial graph holds only some resources (loaded on demand). degree is
   * each loaded resource's total relationship count in the whole graph;
   * totals is the whole graph's size.
   */
  partial?: boolean;
  /** How the graph compares with the catalog it came from (pudl exports). */
  coverage?: Coverage;
  degree?: Record<string, number>;
  /**
   * Partial graphs: for each owner present (account, project), how many
   * resources it owns in the whole graph. Ownership is the `account`
   * attribute, not a relationship, so degree does not count it.
   */
  owned?: Record<string, number>;
  totals?: { resources: number; relationships: number };
}

export interface CanonicalGraph {
  meta: GraphMeta;
  resources: Resource[];
  relationships: Relationship[];
  /** What the types mean, as pudl declares them in CUE. Absent: every type is unknown. */
  vocabulary?: Vocabulary;
}

/**
 * Read-only lookups over a canonical graph. Built once per graph and shared
 * by projections and the inspector.
 */
export class GraphIndex {
  readonly graph: CanonicalGraph;
  /** The graph's vocabulary, indexed. */
  readonly vocab: Vocab;
  private readonly byId = new Map<ResourceId, Resource>();
  private readonly relById = new Map<RelationshipId, Relationship>();
  private readonly outgoing = new Map<ResourceId, Relationship[]>();
  private readonly incoming = new Map<ResourceId, Relationship[]>();
  private readonly byTypeNative = new Map<string, Resource>();

  constructor(graph: CanonicalGraph) {
    this.graph = graph;
    this.vocab = new Vocab(graph.vocabulary);
    for (const r of graph.resources) {
      this.byId.set(r.id, r);
      if (r.nativeId) this.byTypeNative.set(`${r.type}\u0000${r.nativeId}`, r);
    }
    for (const rel of graph.relationships) {
      this.relById.set(rel.id, rel);
      push(this.outgoing, rel.source, rel);
      push(this.incoming, rel.target, rel);
    }
  }

  resource(id: ResourceId): Resource | undefined {
    return this.byId.get(id);
  }

  /** The resource of `type` whose native identifier is `nativeId`, if collected. */
  byNative(type: string, nativeId: string): Resource | undefined {
    return this.byTypeNative.get(`${type}\u0000${nativeId}`);
  }

  has(id: ResourceId): boolean {
    return this.byId.has(id);
  }

  relationship(id: RelationshipId): Relationship | undefined {
    return this.relById.get(id);
  }

  out(id: ResourceId, type?: string): Relationship[] {
    const all = this.outgoing.get(id) ?? [];
    return type ? all.filter((r) => r.type === type) : all;
  }

  in(id: ResourceId, type?: string): Relationship[] {
    const all = this.incoming.get(id) ?? [];
    return type ? all.filter((r) => r.type === type) : all;
  }

  /** Targets of outgoing relationships of `type` that exist in the graph. */
  targets(id: ResourceId, type: string): Resource[] {
    return this.out(id, type)
      .map((r) => this.byId.get(r.target))
      .filter((r): r is Resource => r !== undefined);
  }

  /** Sources of incoming relationships of `type` that exist in the graph. */
  sources(id: ResourceId, type: string): Resource[] {
    return this.in(id, type)
      .map((r) => this.byId.get(r.source))
      .filter((r): r is Resource => r !== undefined);
  }
}

function push<K, V>(m: Map<K, V[]>, k: K, v: V) {
  const list = m.get(k);
  if (list) list.push(v);
  else m.set(k, [v]);
}
