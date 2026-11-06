import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { GraphIndex, type ResourceId } from '../model/canonical';
import { project } from '../projection/project';
import { PROJECTIONS } from '../projection/registry';
import type { ProjectionId, SemanticView } from '../projection/types';
import { GraphCanvas, type CanvasHandle } from '../render/GraphCanvas';
import { computeHighlight, type Selection } from '../render/highlight';
import type { VisualNode, VisualPort } from '../visual/types';
import { initialViewState, revealed, type ViewState } from '../visual/viewState';
import { EmptyViewHint, inView } from './EmptyViewHint';
import { Inspector } from './Inspector';
import { Sidebar } from './Sidebar';
import { StatusBar } from './StatusBar';
import { TimeControl } from './TimeControl';
import { Toolbar } from './Toolbar';
import { useGraphSource } from './useGraphSource';
import { usePipeline } from './usePipeline';

type ViewStates = Partial<Record<ProjectionId, ViewState>>;

/** Below this window width the inspector starts hidden. */
const NARROW_SCREEN = 1200;

export function App() {
  const [projectionId, setProjectionId] = useState<ProjectionId>('network');
  const [viewStates, setViewStates] = useState<ViewStates>({});
  const [selection, setSelection] = useState<Selection>(null);
  const [anchor, setAnchor] = useState<string | undefined>();
  const [centerTarget, setCenterTarget] = useState<ResourceId | null>(null);
  const [layoutNonce, setLayoutNonce] = useState(0);
  const canvas = useRef<CanvasHandle>(null);
  // The inspector starts hidden on narrow screens and opens on selection.
  const [inspectorOpen, setInspectorOpen] = useState(() => window.innerWidth >= NARROW_SCREEN);

  const gs = useGraphSource();
  const { source, loaded, loadError, reloading, reloadError, reload, asOf, autoRefresh } = gs;

  const index = useMemo(() => (loaded ? new GraphIndex(loaded.graph) : null), [loaded]);
  const views = useMemo(() => {
    const out = new Map<ProjectionId, SemanticView>();
    if (index) for (const p of PROJECTIONS) out.set(p.id, project(index, p));
    return out;
  }, [index]);
  const view = views.get(projectionId) ?? null;
  const state = viewStates[projectionId] ?? null;

  useEffect(() => {
    if (view && !viewStates[projectionId]) setViewStates((s) => ({ ...s, [projectionId]: initialViewState(view) }));
  }, [view, projectionId, viewStates]);

  const updateState = useCallback(
    (fn: (s: ViewState) => ViewState, anchorNode?: string) => {
      setAnchor(anchorNode);
      setViewStates((all) => {
        const cur = all[projectionId];
        return cur ? { ...all, [projectionId]: fn(cur) } : all;
      });
    },
    [projectionId],
  );

  const { rendered, pending, error, engineName } = usePipeline(index, view, state, layoutNonce);
  const graph = rendered?.graph ?? null;
  const highlight = useMemo(
    () => (graph ? computeHighlight(graph, selection) : { active: false, nodes: new Set<string>(), edges: new Set<string>() }),
    [graph, selection],
  );
  const selectedNodeId = selection?.kind === 'resource' ? graph?.resourceToNode.get(selection.id) : undefined;
  useEffect(() => {
    if (selection) setInspectorOpen(true);
  }, [selection]);

  // After a layout that may have moved the target, center on it.
  useEffect(() => {
    if (!centerTarget || !graph) return;
    // Wait for a layout that places it: a new graph is drawn on the
    // previous layout until its own arrives.
    const nodeId = graph.resourceToNode.get(centerTarget);
    if (nodeId && canvas.current?.centerOnNode(nodeId)) setCenterTarget(null);
  }, [rendered, centerTarget, graph]);

  const selectResource = useCallback(
    (id: ResourceId, opts: { reveal?: boolean; center?: boolean } = {}) => {
      setSelection({ kind: 'resource', id });
      if (opts.reveal && view && state) {
        const next = revealed(view, state.collapsed, id);
        if (next.size !== state.collapsed.size) updateState((s) => ({ ...s, collapsed: next }));
      }
      if (opts.center) setCenterTarget(id);
    },
    [view, state, updateState],
  );

  const onSelectNode = (n: VisualNode) => {
    if (n.resourceId) setSelection({ kind: 'resource', id: n.resourceId });
    else if (n.groupId) setSelection({ kind: 'group', id: n.groupId });
  };
  const onSelectPort = (p: VisualPort) => p.resourceId && setSelection({ kind: 'resource', id: p.resourceId });
  const toggleGroup = useCallback(
    (groupId: string) =>
      updateState((s) => {
        const collapsed = new Set(s.collapsed);
        if (collapsed.has(groupId)) collapsed.delete(groupId);
        else collapsed.add(groupId);
        return { ...s, collapsed };
      }, groupId),
    [updateState],
  );

  const switchProjection = (id: ProjectionId) => {
    setProjectionId(id);
    setAnchor(undefined);
    if (selection?.kind === 'resource') setCenterTarget(selection.id);
    else setSelection(null);
  };

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement).closest('input, textarea, select')) return;
      if (e.key === 'f') canvas.current?.fit();
      else if (e.key === 'Escape') setSelection(null);
      else if (e.key === '+' || e.key === '=') canvas.current?.zoomBy(1.25);
      else if (e.key === '-') canvas.current?.zoomBy(0.8);
      else if (e.key === 'i' && !e.metaKey && !e.ctrlKey && !e.altKey) setInspectorOpen((o) => !o);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  if (loadError) return <div className="fatal">Failed to load graph: {loadError}</div>;
  if (!loaded || !index || !view || !state) return <div className="fatal">Loading…</div>;

  return (
    <div className={`app${inspectorOpen ? '' : ' inspector-closed'}`}>
      <Sidebar
        meta={loaded.graph.meta}
        onReload={loaded.graph.meta.origin === 'mock' ? undefined : reload}
        reloading={reloading}
        reloadError={reloadError}
        autoRefresh={source?.version && !asOf ? autoRefresh : undefined}
        onAutoRefresh={gs.setAutoRefresh}
        timeControl={source?.snapshots ? <TimeControl source={source} asOf={asOf} onAsOf={gs.setAsOf} /> : undefined}
        index={index}
        views={views}
        projectionId={projectionId}
        state={state}
        selection={selection}
        onProjection={switchProjection}
        onSelectResource={(id) => selectResource(id, { reveal: true, center: true })}
        remoteSearch={gs.search}
        onPickRemote={(id) => {
          gs.expand([id]);
          selectResource(id, { reveal: true, center: true });
        }}
        onSelectGroup={(gid) => setSelection({ kind: 'group', id: gid })}
        onToggleGroup={toggleGroup}
        onState={(fn) => updateState(fn, selectedNodeId)}
      />
      <main className="workspace">
        <Toolbar
          view={view}
          state={state}
          selection={selection}
          pending={pending}
          onState={(fn) => updateState(fn, selectedNodeId)}
          onFit={() => canvas.current?.fit()}
          onZoom={(f) => canvas.current?.zoomBy(f)}
          onRelayout={() => {
            setAnchor(undefined);
            setLayoutNonce((n) => n + 1);
          }}
        />
        <div className="canvas-wrap">
          {rendered ? (
            <GraphCanvas
              ref={canvas}
              graph={rendered.graph}
              layout={rendered.layout}
              highlight={highlight}
              anchorNodeId={anchor ?? selectedNodeId}
              fitKey={`${rendered.viewKey}#${layoutNonce}`}
              onSelectNode={onSelectNode}
              onSelectPort={onSelectPort}
              onSelectEdge={(id) => setSelection({ kind: 'edge', id })}
              onToggleGroup={(n) => n.groupId && toggleGroup(n.groupId)}
              onExpand={
                gs.partial
                  ? (n) => {
                      if (!n.resourceId) return;
                      setAnchor(n.id); // keep the expanded node where it is while it grows
                      gs.expand([n.resourceId]);
                    }
                  : undefined
              }
              onClear={() => setSelection(null)}
            />
          ) : (
            <div className="canvas-empty">Laying out…</div>
          )}
          {error && <div className="canvas-error">Layout failed: {error}</div>}
          {rendered && rendered.graph.nodes.size === 0 && loaded.graph.resources.length > 0 && !(gs.partial && !gs.expanded) && (
            <EmptyViewHint
              view={view}
              views={views}
              loadedCount={loaded.graph.resources.length}
              partial={gs.partial}
              selected={selection?.kind === 'resource' ? index.resource(selection.id)?.name ?? selection.id : undefined}
              selectedIn={selection?.kind === 'resource' ? PROJECTIONS.filter((p) => inView(views.get(p.id), selection.id)) : []}
              onProjection={switchProjection}
            />
          )}
          {gs.partial && !gs.expanded && (
            <div className={`canvas-hint${loaded.graph.resources.length > 0 ? ' top' : ''}`}>
              <div>
                <strong>Large inventory: {loaded.graph.meta.totals?.resources ?? 'many'} resources.</strong>{' '}
                {loaded.graph.resources.length > 0 ? 'Showing accounts and projects only. Click a ⋯ badge to load what one holds, or ' : ''}
                {loaded.graph.resources.length > 0 ? 'search' : 'Search'} the whole inventory on the left to load a resource and its
                neighbours. Open with <code>?full</code> to load everything at once.
              </div>
            </div>
          )}
        </div>
        <StatusBar
          rendered={rendered}
          view={view}
          issues={loaded.issues}
          warnings={loaded.graph.meta.warnings}
          loadedCount={gs.partial ? loaded.graph.resources.length : undefined}
          totals={gs.partial ? loaded.graph.meta.totals : undefined}
          engineName={engineName}
        />
      </main>
      <Inspector
        open={inspectorOpen}
        onToggle={() => setInspectorOpen((o) => !o)}
        index={index}
        views={views}
        projectionId={projectionId}
        graph={graph}
        state={state}
        selection={selection}
        onSelectResource={(id) => selectResource(id, { reveal: true, center: true })}
        onSelectEdge={(id) => setSelection({ kind: 'edge', id })}
        onProjection={switchProjection}
        onToggleGroup={toggleGroup}
        onFocus={(id, depth) => updateState((s) => ({ ...s, focus: depth > 0 ? { resourceId: id, depth } : null }))}
        onExpand={
          gs.partial
            ? (id) => {
                setAnchor(graph?.resourceToNode.get(id));
                gs.expand([id]);
              }
            : undefined
        }
      />
    </div>
  );
}
