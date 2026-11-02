import type { ProjectionDefinition, ProjectionId } from './types';
import { applicationProjection } from './views/application';
import { inventoryProjection } from './views/inventory';
import { networkProjection } from './views/network';
import { securityProjection } from './views/security';

export const PROJECTIONS: ProjectionDefinition[] = [
  networkProjection,
  applicationProjection,
  inventoryProjection,
  securityProjection,
];

export function projectionById(id: ProjectionId): ProjectionDefinition {
  const p = PROJECTIONS.find((d) => d.id === id);
  if (!p) throw new Error(`unknown projection ${id}`);
  return p;
}
