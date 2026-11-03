// Visual vocabulary for relationship categories. Each category has its own
// colour AND line pattern, so they stay distinguishable without colour.

import type { RelationshipCategory } from '../model/vocabulary';
import type { Point } from '../layout/types';

export interface EdgeStyle {
  color: string;
  dash?: string;
  width: number;
}

export const EDGE_STYLES: Record<RelationshipCategory, EdgeStyle> = {
  network: { color: 'var(--cat-network)', width: 1.5 },
  traffic: { color: 'var(--cat-traffic)', width: 2.25 },
  dependency: { color: 'var(--cat-dependency)', width: 1.5, dash: '10 3' },
  security: { color: 'var(--cat-security)', width: 1.4, dash: '5 4' },
  iam: { color: 'var(--cat-iam)', width: 1.6, dash: '1.5 3.5' },
  containment: { color: 'var(--cat-containment)', width: 1.1, dash: '3 3' },
  other: { color: 'var(--cat-other)', width: 1.2, dash: '8 3 2 3' },
};

export function pathFor(sections: Point[][]): string {
  return sections
    .map((pts) => pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' '))
    .join(' ');
}

/** Midpoint of the longest segment: a stable, readable place for a label. */
export function labelPoint(sections: Point[][]): Point | undefined {
  let best: { len: number; at: Point } | undefined;
  for (const pts of sections) {
    for (let i = 1; i < pts.length; i++) {
      const a = pts[i - 1];
      const b = pts[i];
      const len = Math.hypot(b.x - a.x, b.y - a.y);
      if (!best || len > best.len) best = { len, at: { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 } };
    }
  }
  return best?.at;
}
