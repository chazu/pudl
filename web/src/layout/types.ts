// Layout-engine-neutral types. Projections pick a LayoutConfig; the engine
// adapter (elkAdapter.ts) is the only code that knows about ELK.

import type { VisualGraph } from '../visual/types';

export type Direction = 'RIGHT' | 'DOWN';

export interface LayoutConfig {
  /** 'layered' routes edges; 'packed' arranges groups compactly and draws edges straight. */
  algorithm: 'layered' | 'packed';
  direction: Direction;
}

export interface Point {
  x: number;
  y: number;
}

export interface Box {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface EdgeRoute {
  /** One polyline per section, absolute coordinates. */
  sections: Point[][];
  /** Where to put the edge label, if the engine reserved space for one. */
  labelAt?: Point;
  /** False when the engine did not route the edge; the renderer draws a straight line. */
  routed: boolean;
}

export interface LayoutResult {
  nodes: Map<string, Box>;
  ports: Map<string, Box>;
  edges: Map<string, EdgeRoute>;
  bounds: Box;
  durationMs: number;
}

/** Positions from a previous layout, used to keep placement stable. */
export interface LayoutHints {
  previous?: LayoutResult;
}

export interface LayoutEngine {
  readonly name: string;
  layout(graph: VisualGraph, config: LayoutConfig, hints: LayoutHints): Promise<LayoutResult>;
}
