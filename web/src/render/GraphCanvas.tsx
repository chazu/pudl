// Interactive SVG canvas: pan, zoom, select, collapse. Renders a visual
// graph with a layout result; holds no graph semantics of its own.

import { forwardRef, useCallback, useEffect, useImperativeHandle, useLayoutEffect, useRef, useState } from 'react';
import type { LayoutResult } from '../layout/types';
import { CATEGORY_IDS } from '../model/vocabulary';
import type { VisualGraph, VisualNode, VisualPort } from '../visual/types';
import { EDGE_STYLES, labelPoint, pathFor } from './edgeStyle';
import type { Highlight } from './highlight';
import { CollapsedShape, GroupShape, LeafShape, PortShape } from './NodeShapes';
import { centerOn, compensate, fitBox, zoomAt, type Viewport } from './viewport';

export interface CanvasHandle {
  fit(): void;
  zoomBy(factor: number): void;
  centerOnNode(nodeId: string): void;
}

interface Props {
  graph: VisualGraph;
  layout: LayoutResult;
  highlight: Highlight;
  /** Visual node to keep fixed on screen when the layout changes. */
  anchorNodeId?: string;
  /** Changes when the canvas should fit instead of anchoring (e.g. view switch). */
  fitKey: string;
  onSelectNode(node: VisualNode): void;
  onSelectPort(port: VisualPort): void;
  onSelectEdge(edgeId: string): void;
  onToggleGroup(node: VisualNode): void;
  /** Partial graphs: load the neighbours of a node's resource. */
  onExpand?(node: VisualNode): void;
  onClear(): void;
}

export const GraphCanvas = forwardRef<CanvasHandle, Props>(function GraphCanvas(props, ref) {
  const { graph, layout, highlight, anchorNodeId, fitKey } = props;
  const svgRef = useRef<SVGSVGElement>(null);
  const [vp, setVp] = useState<Viewport>({ x: 0, y: 0, k: 1 });
  const prevLayout = useRef<LayoutResult | null>(null);
  const prevFitKey = useRef<string | null>(null);
  const drag = useRef<{ x: number; y: number; vx: number; vy: number; moved: boolean } | null>(null);
  const justDragged = useRef(false);

  const size = () => {
    const r = svgRef.current?.getBoundingClientRect();
    return { w: r?.width ?? 800, h: r?.height ?? 600 };
  };

  const fit = useCallback(() => {
    const { w, h } = size();
    setVp(fitBox(layout.bounds, w, h));
  }, [layout]);

  useImperativeHandle(ref, () => ({
    fit,
    zoomBy(factor) {
      const { w, h } = size();
      setVp((v) => zoomAt(v, factor, { x: w / 2, y: h / 2 }));
    },
    centerOnNode(nodeId) {
      const box = layout.nodes.get(nodeId);
      if (!box) return;
      const { w, h } = size();
      setVp((v) => centerOn(v, box, w, h));
    },
  }));

  // On a new layout: fit when the view changed, otherwise keep the anchor still.
  useLayoutEffect(() => {
    const before = prevLayout.current;
    prevLayout.current = layout;
    if (prevFitKey.current !== fitKey || !before) {
      prevFitKey.current = fitKey;
      fit();
      return;
    }
    if (anchorNodeId) {
      const a = before.nodes.get(anchorNodeId);
      const b = layout.nodes.get(anchorNodeId);
      if (a && b) setVp((v) => compensate(v, a, b));
    }
  }, [layout, fitKey, anchorNodeId, fit]);

  // Wheel zoom (non-passive so we can prevent page scroll).
  useEffect(() => {
    const el = svgRef.current;
    if (!el) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const r = el.getBoundingClientRect();
      const factor = Math.exp(-e.deltaY * (e.ctrlKey ? 0.01 : 0.0015));
      setVp((v) => zoomAt(v, factor, { x: e.clientX - r.left, y: e.clientY - r.top }));
    };
    el.addEventListener('wheel', onWheel, { passive: false });
    return () => el.removeEventListener('wheel', onWheel);
  }, []);

  const onPointerDown = (e: React.PointerEvent) => {
    if (e.button !== 0) return;
    justDragged.current = false;
    drag.current = { x: e.clientX, y: e.clientY, vx: vp.x, vy: vp.y, moved: false };
  };
  const onPointerMove = (e: React.PointerEvent) => {
    const d = drag.current;
    if (!d) return;
    const dx = e.clientX - d.x;
    const dy = e.clientY - d.y;
    if (!d.moved && Math.hypot(dx, dy) < 3) return;
    d.moved = true;
    setVp((v) => ({ ...v, x: d.vx + dx, y: d.vy + dy }));
  };
  const onPointerUp = () => {
    if (drag.current?.moved) justDragged.current = true;
    drag.current = null;
  };
  // Clicks that end a drag must not select or clear (click fires after pointerup).
  const guard = <T,>(fn: (arg: T) => void) => (arg: T) => {
    if (justDragged.current) return;
    fn(arg);
  };

  const depthOf = (id: string): number => {
    let d = 0;
    let cur = graph.nodes.get(id)?.parentId;
    while (cur) {
      d++;
      cur = graph.nodes.get(cur)?.parentId;
    }
    return d;
  };
  const ordered = [...graph.nodes.values()].sort((a, b) => depthOf(a.id) - depthOf(b.id));
  const groups = ordered.filter((n) => n.kind === 'group');
  const leaves = ordered.filter((n) => n.kind !== 'group');
  const dim = (id: string) => highlight.active && !highlight.nodes.has(id);
  const dimEdge = (id: string) => highlight.active && !highlight.edges.has(id);

  return (
    <svg
      ref={svgRef}
      className="canvas"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerLeave={onPointerUp}
      onClick={guard(() => props.onClear())}
    >
      <defs>
        {CATEGORY_IDS.map((c) => (
          <marker key={c} id={`arrow-${c}`} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
            <path d="M0,1 L9,5 L0,9 z" fill={EDGE_STYLES[c].color} />
          </marker>
        ))}
      </defs>
      <g transform={`translate(${vp.x},${vp.y}) scale(${vp.k})`}>
        {groups.map((n) => {
          const box = layout.nodes.get(n.id);
          if (!box) return null;
          return <GroupShape key={n.id} node={n} box={box} dimmed={false} depth={depthOf(n.id)} selected={highlight.primaryNode === n.id} onSelect={guard(props.onSelectNode)} onToggle={props.onToggleGroup} onExpand={props.onExpand} />;
        })}
        <g className="edges">
          {graph.edges.map((e) => {
            const route = layout.edges.get(e.id);
            if (!route) return null;
            const style = EDGE_STYLES[e.category];
            const d = pathFor(route.sections);
            const active = highlight.edges.has(e.id);
            const at = route.labelAt ?? labelPoint(route.sections);
            const showLabel = at && (e.aggregated || active);
            return (
              <g key={e.id} className={`vedge cat-${e.category}${dimEdge(e.id) ? ' dimmed' : ''}${active ? ' active' : ''}${highlight.primaryEdge === e.id ? ' selected' : ''}`} onClick={(ev) => { ev.stopPropagation(); guard(props.onSelectEdge)(e.id); }}>
                <path className="hit" d={d} />
                <path className="line" d={d} stroke={style.color} strokeWidth={e.aggregated ? style.width + 1 : style.width} strokeDasharray={style.dash} markerEnd={`url(#arrow-${e.category})`} />
                {showLabel && (
                  <g className={`elabel${e.aggregated ? ' agg' : ''}`} transform={`translate(${at.x},${at.y})`}>
                    <rect x={-(e.label.length * 3.3 + 6)} y={-8} width={e.label.length * 6.6 + 12} height={16} rx={8} stroke={style.color} />
                    <text textAnchor="middle" y={4}>{e.label}</text>
                  </g>
                )}
                <title>{`${e.label}${e.aggregated ? ` (${e.relationshipIds.length} relationships)` : ''}`}</title>
              </g>
            );
          })}
        </g>
        {leaves.map((n) => {
          const box = layout.nodes.get(n.id);
          if (!box) return null;
          const common = { node: n, box, dimmed: dim(n.id), depth: depthOf(n.id), selected: highlight.primaryNode === n.id, onSelect: guard(props.onSelectNode), onToggle: props.onToggleGroup, onExpand: props.onExpand };
          return n.kind === 'collapsed' ? <CollapsedShape key={n.id} {...common} /> : <LeafShape key={n.id} {...common} />;
        })}
        {ordered.flatMap((n) =>
          n.ports.map((p) => {
            const box = layout.ports.get(p.id);
            return box ? <PortShape key={p.id} port={p} box={box} dimmed={dim(n.id)} onSelect={guard(props.onSelectPort)} /> : null;
          }),
        )}
      </g>
    </svg>
  );
});
