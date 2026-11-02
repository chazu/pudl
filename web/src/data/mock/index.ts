// Mock demonstration source. Clearly separated from any live inventory:
// meta.origin is always 'mock' and the UI shows a persistent banner.

import type { CanonicalGraph } from '../../model/canonical';
import type { GraphSource } from '../source';
import { buildApplications } from './applications';
import { buildAws } from './aws';
import { GraphBuilder } from './builder';
import { buildGcp } from './gcp';

/** Deterministic: the same graph, in the same order, every call. */
export function buildDemoGraph(): CanonicalGraph {
  const aws = buildAws(new GraphBuilder('aws'));
  const gcp = buildGcp(new GraphBuilder('gcp'));
  const apps = buildApplications(new GraphBuilder('logical'));
  // Cross-provider dependency: the orders worker exports to the analytics bucket.
  apps.rel('writes_to', 'aws:i-0wrk1', 'gcp:bucket:acme-analytics-raw', 'declared', {
    evidence: [{ basis: 'declared', source: 'orders/deploy/services.yaml', detail: 'EXPORT_BUCKET=gs://acme-analytics-raw' }],
  });
  return {
    meta: { origin: 'mock', label: 'Demo dataset (synthetic)', generatedAt: '2026-10-30T14:00:00Z' },
    resources: [...aws.resources, ...gcp.resources, ...apps.resources],
    relationships: [...aws.relationships, ...gcp.relationships, ...apps.relationships],
  };
}

export class MockGraphSource implements GraphSource {
  readonly id = 'mock';
  readonly label = 'Demo dataset';

  async load(): Promise<CanonicalGraph> {
    return buildDemoGraph();
  }
}
