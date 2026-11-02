// The layout engine the UI uses. Layered views use the incremental ELK
// engine (stable under collapse, expand and refresh; see elkIncremental.ts);
// the packed inventory view, and `?engine=classic`, use the single-pass ELK
// adapter (elkAdapter.ts).

import { ElkLayoutEngine } from './elkAdapter';
import { ElkIncrementalEngine } from './elkIncremental';
import type { LayoutConfig, LayoutEngine, LayoutHints, LayoutResult } from './types';
import type { VisualGraph } from '../visual/types';

export function createLayoutEngine(kind: 'incremental' | 'classic' = 'incremental'): LayoutEngine {
  const classic = new ElkLayoutEngine();
  const incremental = new ElkIncrementalEngine();
  return {
    name: kind === 'classic' ? classic.name : incremental.name,
    layout(graph: VisualGraph, config: LayoutConfig, hints: LayoutHints): Promise<LayoutResult> {
      const engine = kind === 'classic' || config.algorithm === 'packed' ? classic : incremental;
      return engine.layout(graph, config, hints);
    },
  };
}
