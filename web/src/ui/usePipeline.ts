// The transformation pipeline as a hook:
//   canonical graph → semantic view (projection) → visual graph → layout.
//
// Two visual graphs are built from the same semantic view: the *layout*
// graph (filters neutralised unless reflowOnFilter is on) and the *display*
// graph (everything the user asked for). The display graph is a subset of
// the layout graph, so it can be drawn at the layout graph's positions; a
// filter change therefore rebuilds only the display graph and moves nothing.
//
// Layout is async. Until it finishes, the previous (display, layout) pair
// stays on screen, so a frame never mixes a graph with foreign positions.

import { useEffect, useMemo, useState } from 'react';
import { ElkLayoutEngine } from '../layout/elkAdapter';
import { LayoutMemory } from '../layout/layoutMemory';
import type { LayoutEngine, LayoutResult } from '../layout/types';
import type { GraphIndex } from '../model/canonical';
import type { SemanticView } from '../projection/types';
import { buildVisualGraph } from '../visual/build';
import type { VisualGraph } from '../visual/types';
import { layoutKey, layoutStateOf, type ViewState } from '../visual/viewState';

export interface Rendered {
  graph: VisualGraph;
  layout: LayoutResult;
  /** Projection + direction: changes mean "a different picture", so the canvas refits. */
  viewKey: string;
}

const engine: LayoutEngine = new ElkLayoutEngine();
const memory = new LayoutMemory();

interface LayoutRun {
  key: string;
  layout: LayoutResult;
}

export function usePipeline(index: GraphIndex | null, view: SemanticView | null, state: ViewState | null, layoutNonce: number) {
  const layoutState = useMemo(() => (state && view ? layoutStateOf(state, view) : null), [state, view]);
  const key = layoutState && view ? `${view.projection.id}|${layoutKey(layoutState)}|${layoutNonce}` : null;

  // Rebuilt only when the layout-relevant state changes.
  const layoutGraph = useMemo(
    () => (index && view && layoutState ? buildVisualGraph(view, index, layoutState) : null),
    [index, view, key],
  );
  const display = useMemo(() => (index && view && state ? buildVisualGraph(view, index, state) : null), [index, view, state]);

  const [run, setRun] = useState<LayoutRun | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!layoutGraph || !view || !layoutState || !key) return;
    let cancelled = false;
    setPending(true);
    const memKey = `${view.projection.id}:${layoutState.direction}`;
    engine
      .layout(layoutGraph, { ...view.projection.layout, direction: layoutState.direction }, { previous: memory.previous(memKey) })
      .then((layout) => {
        if (cancelled) return;
        memory.remember(memKey, layout);
        setRun({ key, layout });
        setError(null);
      })
      .catch((e: unknown) => !cancelled && setError(String(e)))
      .finally(() => !cancelled && setPending(false));
    return () => {
      cancelled = true;
    };
  }, [layoutGraph]);

  // Pair the display graph with a layout only when the layout belongs to it.
  const [rendered, setRendered] = useState<Rendered | null>(null);
  useEffect(() => {
    if (display && run && run.key === key && view && state) {
      setRendered({ graph: display, layout: run.layout, viewKey: `${view.projection.id}:${state.direction}` });
    }
  }, [display, run, key, view, state]);

  return { rendered, pending, error, engineName: engine.name };
}
