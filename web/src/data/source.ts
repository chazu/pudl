// Backend integration point. The UI only ever talks to a GraphSource:
//   LiveGraphSource  the catalog, via the `pudl ui` server (origin 'live')
//   JsonFileSource   a `pudl graph export` file or any graph JSON (?graph=<url>)
//   MockGraphSource  the synthetic demo dataset (?demo, or no pudl server)
// See docs/frontend.md.

import type { CanonicalGraph } from '../model/canonical';
import { validateGraph, type GraphIssue } from './validate';

export interface LoadOptions {
  /** RFC 3339: the graph as recorded at this time (sources with history only). */
  asOf?: string;
}

/** An observation snapshot, offered as a point to view the graph at. */
export interface SnapshotInfo {
  id: string;
  createdAt: string;
  model?: string;
  scope?: string;
  complete: boolean;
}

/** A lightweight search result from a source that can search without loading. */
export interface SearchHit {
  id: string;
  name: string;
  type: string;
  provider: string;
  account?: string;
  region?: string;
  nativeId?: string;
}

export interface GraphSummary {
  resources: number;
  relationships: number;
}

export interface GraphSource {
  readonly id: string;
  readonly label: string;
  load(opts?: LoadOptions): Promise<CanonicalGraph>;
  /** Current change stamp, for sources that can tell when to reload (live only). */
  version?(): Promise<string | null>;
  /**
   * Push: call onVersion with the change stamp on connect and on every change;
   * onFail if the stream is unavailable (callers fall back to version()).
   * Returns an unsubscribe function.
   */
  subscribe?(onVersion: (version: string) => void, onFail: () => void): () => void;
  /** Recent snapshots, newest first, for sources with history (live only). */
  snapshots?(): Promise<SnapshotInfo[]>;
  /** Size of the whole graph, without loading it (live only). */
  summary?(opts?: LoadOptions): Promise<GraphSummary>;
  /** The administrative resources (organizations, folders, accounts, projects), following `closure` (live only). */
  overview?(closure: string[], opts?: LoadOptions): Promise<CanonicalGraph>;
  /** Owner resources (`ids`) and every resource they own, following `closure` (live only). */
  owned?(ids: string[], closure: string[], opts?: LoadOptions): Promise<CanonicalGraph>;
  /** A partial graph around `ids`, following `closure` types outward (live only). */
  neighbourhood?(ids: string[], closure: string[], opts?: LoadOptions): Promise<CanonicalGraph>;
  /** Search the whole graph without loading it (live only). */
  search?(q: string, opts?: LoadOptions): Promise<SearchHit[]>;
}

export interface LoadedGraph {
  graph: CanonicalGraph;
  issues: GraphIssue[];
}

export async function loadFrom(source: GraphSource, opts?: LoadOptions): Promise<LoadedGraph> {
  const graph = await source.load(opts);
  return { graph, issues: validateGraph(graph) };
}

/**
 * Pick a source: `?graph=<url>` loads that file, `?demo` the demo dataset;
 * otherwise the live catalog when `pudl ui` served the page, else the demo.
 * Every origin is labelled in the UI.
 */
export async function sourceFromLocation(search: string): Promise<GraphSource> {
  const params = new URLSearchParams(search);
  const url = params.get('graph');
  if (url) {
    const { JsonFileSource } = await import('./jsonSource');
    return new JsonFileSource(url);
  }
  if (!params.has('demo')) {
    const { LiveGraphSource, servedByPudl } = await import('./liveSource');
    if (await servedByPudl()) return new LiveGraphSource();
  }
  const { MockGraphSource } = await import('./mock');
  return new MockGraphSource();
}
