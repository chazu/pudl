// Small helpers for writing the demo dataset tersely and deterministically.

import type {
  Basis,
  Evidence,
  Relationship,
  Resource,
  Provider,
} from '../../model/canonical';

export const DEMO_OBSERVED_AT = '2026-10-30T14:00:00Z';

export function observed(source: string, detail?: string): Evidence {
  return { basis: 'observed', source, observedAt: DEMO_OBSERVED_AT, snapshotId: 'demo-snapshot-1', detail };
}

export function declared(source: string, detail?: string): Evidence {
  return { basis: 'declared', source, detail };
}

export function inferred(source: string, detail?: string): Evidence {
  return { basis: 'inferred', source, detail };
}

type ResourceInput = Omit<Resource, 'properties' | 'provider'> & {
  provider?: Provider;
  properties?: Record<string, unknown>;
};

export class GraphBuilder {
  readonly resources: Resource[] = [];
  readonly relationships: Relationship[] = [];
  private relCount = new Map<string, number>();

  constructor(private readonly provider: Provider) {}

  res(input: ResourceInput): string {
    this.resources.push({
      provider: this.provider,
      properties: {},
      ...input,
    });
    return input.id;
  }

  /**
   * Add a relationship. IDs are derived from (type, source, target) so the
   * dataset is deterministic; repeats get a numeric suffix.
   */
  rel(
    type: string,
    source: string,
    target: string,
    basis: Basis,
    opts: { properties?: Record<string, unknown>; evidence?: Evidence[] } = {},
  ): string {
    const base = `${type}:${source}->${target}`;
    const n = this.relCount.get(base) ?? 0;
    this.relCount.set(base, n + 1);
    const id = n === 0 ? base : `${base}#${n + 1}`;
    this.relationships.push({ id, type, source, target, basis, ...opts });
    return id;
  }
}
