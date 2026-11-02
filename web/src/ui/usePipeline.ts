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

import { useEffect, useMemo, useRef, useState } from 'react';
import { createLayoutEngine } from '../layout/engine';
import { LayoutMemory, layoutSignature } from '../layout/layoutMemory';
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

const engine: LayoutEngine = createLayoutEngine(new URLSearchParams(window.location.search).get('engine') === 'classic' ? 'classic' : 'incremental');
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

  const lastNonce = useRef(layoutNonce);
  useEffect(() => {
    if (!layoutGraph || !view || !layoutState || !key) return;
    let cancelled = false;
    const memKey = `${view.projection.id}:${layoutState.direction}`;
    if (lastNonce.current !== layoutNonce) {
      // Relayout: start from scratch, without previous positions or cached layouts.
      memory.forget(memKey);
      lastNonce.current = layoutNonce;
    }
    const config = { ...view.projection.layout, direction: layoutState.direction };
    const signature = layoutSignature(layoutGraph, config);
    const cached = memory.cached(memKey, signature);
    if (cached) {
      // A state laid out before (e.g. collapse then expand): reproduce it exactly.
      memory.remember(memKey, cached, signature);
      setRun({ key, layout: cached });
      return;
    }
    setPending(true);
    engine
      .layout(layoutGraph, config, { previous: memory.previous(memKey) })
      .then((layout) => {
        if (cancelled) return;
        memory.remember(memKey, layout, signature);
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
