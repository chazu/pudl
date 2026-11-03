// Structural checks on an incoming canonical graph. Issues are reported, not
// repaired: a relationship to a missing resource stays in the graph and the
// projection shows it as an unresolved reference.

import type { CanonicalGraph, GraphMeta } from '../model/canonical';
import { Vocab, type Vocabulary } from '../model/vocabulary';

export type GraphIssueKind =
  | 'duplicate-resource'
  | 'duplicate-relationship'
  | 'dangling-source'
  | 'dangling-target'
  | 'unknown-relationship-type'
  | 'no-vocabulary';

export interface GraphIssue {
  kind: GraphIssueKind;
  id: string;
  message: string;
}

export function validateGraph(graph: CanonicalGraph): GraphIssue[] {
  const issues: GraphIssue[] = [];
  const vocab = graph.vocabulary ? new Vocab(graph.vocabulary) : null;
  if (!vocab && graph.resources.length > 0) {
    issues.push({
      kind: 'no-vocabulary',
      id: 'vocabulary',
      message: 'the graph carries no vocabulary: every resource type is unknown and every relationship is shown as "other"',
    });
  }
  const ids = new Set<string>();
  for (const r of graph.resources) {
    if (ids.has(r.id)) {
      issues.push({ kind: 'duplicate-resource', id: r.id, message: `resource ${r.id} appears twice` });
    }
    ids.add(r.id);
  }
  const relIds = new Set<string>();
  for (const rel of graph.relationships) {
    if (relIds.has(rel.id)) {
      issues.push({
        kind: 'duplicate-relationship',
        id: rel.id,
        message: `relationship ${rel.id} appears twice`,
      });
    }
    relIds.add(rel.id);
    if (!ids.has(rel.source)) {
      issues.push({
        kind: 'dangling-source',
        id: rel.id,
        message: `${rel.type} source ${rel.source} is not in the inventory`,
      });
    }
    if (!ids.has(rel.target)) {
      issues.push({
        kind: 'dangling-target',
        id: rel.id,
        message: `${rel.type} target ${rel.target} is not in the inventory`,
      });
    }
    if (vocab && !vocab.isKnownRelationshipType(rel.type)) {
      issues.push({
        kind: 'unknown-relationship-type',
        id: rel.id,
        message: `relationship type ${rel.type} is not declared in the vocabulary (declare it with a graph_edge_type fact)`,
      });
    }
  }
  return issues;
}

/**
 * Shape check for JSON input. Throws on anything that is not a graph. The
 * origin comes from the source that fetched it, never from the document: a
 * file cannot claim to be live.
 */
export function parseCanonicalGraph(data: unknown, fallbackLabel: string, origin: 'file' | 'live' = 'file'): CanonicalGraph {
  if (!data || typeof data !== 'object') throw new Error('graph JSON must be an object');
  const obj = data as Record<string, unknown>;
  if (!Array.isArray(obj.resources)) throw new Error('graph JSON needs a resources array');
  if (!Array.isArray(obj.relationships)) throw new Error('graph JSON needs a relationships array');
  for (const r of obj.resources as Record<string, unknown>[]) {
    if (typeof r.id !== 'string' || typeof r.type !== 'string') {
      throw new Error('every resource needs string id and type');
    }
    r.name ??= r.id;
    r.provider ??= 'external';
    r.properties ??= {};
  }
  for (const rel of obj.relationships as Record<string, unknown>[]) {
    for (const k of ['id', 'type', 'source', 'target']) {
      if (typeof rel[k] !== 'string') throw new Error(`every relationship needs string ${k}`);
    }
    rel.basis ??= 'unknown';
  }
  const meta = (obj.meta as Record<string, unknown> | undefined) ?? {};
  const vocabulary = parseVocabulary(obj.vocabulary);
  return {
    meta: {
      origin,
      label: typeof meta.label === 'string' ? meta.label : fallbackLabel,
      generatedAt: typeof meta.generatedAt === 'string' ? meta.generatedAt : undefined,
      version: typeof meta.version === 'string' ? meta.version : undefined,
      asOf: typeof meta.asOf === 'string' ? meta.asOf : undefined,
      partial: meta.partial === true ? true : undefined,
      coverage: meta.coverage && typeof meta.coverage === 'object' ? (meta.coverage as GraphMeta['coverage']) : undefined,
      degree: meta.degree && typeof meta.degree === 'object' ? (meta.degree as Record<string, number>) : undefined,
      totals: meta.totals && typeof meta.totals === 'object' ? (meta.totals as { resources: number; relationships: number }) : undefined,
      warnings: Array.isArray(meta.warnings) ? meta.warnings.filter((w): w is string => typeof w === 'string') : undefined,
    },
    resources: obj.resources as CanonicalGraph['resources'],
    relationships: obj.relationships as CanonicalGraph['relationships'],
    ...(vocabulary ? { vocabulary } : {}),
  };
}

/**
 * Shape check for the vocabulary. Malformed entries are dropped (the types
 * they described then render as unknown); a vocabulary that is not an object
 * is treated as absent.
 */
export function parseVocabulary(data: unknown): Vocabulary | undefined {
  if (!data || typeof data !== 'object') return undefined;
  const v = data as Record<string, unknown>;
  const list = (k: string) => (Array.isArray(v[k]) ? (v[k] as Record<string, unknown>[]).filter((x) => x && typeof x === 'object') : []);
  const str = (x: unknown) => typeof x === 'string';
  return {
    categories: list('categories')
      .filter((c) => str(c.id))
      .map((c) => ({ id: c.id as string, label: str(c.label) ? (c.label as string) : (c.id as string), meaning: str(c.meaning) ? (c.meaning as string) : '', order: typeof c.order === 'number' ? c.order : 0 })),
    edgeTypes: list('edgeTypes')
      .filter((e) => str(e.type) && str(e.category))
      .map((e) => e as unknown as Vocabulary['edgeTypes'][number]),
    providers: list('providers')
      .filter((p) => str(p.provider) && str(p.label))
      .map((p) => p as unknown as Vocabulary['providers'][number]),
    resourceTypes: list('resourceTypes')
      .filter((t) => str(t.type))
      .map((t) => t as unknown as Vocabulary['resourceTypes'][number]),
    problems: Array.isArray(v.problems) ? v.problems.filter(str) as string[] : [],
  };
}
