// Backend integration point. The UI only ever talks to a GraphSource:
//   LiveGraphSource  the catalog, via the `pudl ui` server (origin 'live')
//   JsonFileSource   a `pudl graph export` file or any graph JSON (?graph=<url>)
//   MockGraphSource  the synthetic demo dataset (?demo, or no pudl server)
// See docs/frontend.md.

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
