// Backend integration point.
//
// The UI only ever talks to a GraphSource. PUDL has no HTTP API today (it is a
// CLI over a local SQLite catalog), so the shipped sources are the mock demo
// dataset and a JSON file loader. A live source would implement this
// interface, set meta.origin = 'live', and return a CanonicalGraph built from
// catalog entries (resources) and facts (relationships). See docs/frontend.md.

import type { CanonicalGraph } from '../model/canonical';
import { validateGraph, type GraphIssue } from './validate';

export interface GraphSource {
  readonly id: string;
  readonly label: string;
  load(): Promise<CanonicalGraph>;
}

export interface LoadedGraph {
  graph: CanonicalGraph;
  issues: GraphIssue[];
}

export async function loadFrom(source: GraphSource): Promise<LoadedGraph> {
  const graph = await source.load();
  return { graph, issues: validateGraph(graph) };
}

/**
 * Pick a source from the page URL. `?graph=<url>` loads a canonical graph JSON
 * file; anything else uses the mock dataset. Mock and file data are always
 * labelled as such in the UI.
 */
export async function sourceFromLocation(search: string): Promise<GraphSource> {
  const params = new URLSearchParams(search);
  const url = params.get('graph');
  if (url) {
    const { JsonFileSource } = await import('./jsonSource');
    return new JsonFileSource(url);
  }
  const { MockGraphSource } = await import('./mock');
  return new MockGraphSource();
}
