import type { CanonicalGraph } from '../model/canonical';
import type { GraphSource } from './source';
import { parseCanonicalGraph } from './validate';

/** Loads a canonical graph JSON document (see docs/frontend.md for the shape). */
export class JsonFileSource implements GraphSource {
  readonly id = 'file';
  readonly label: string;

  constructor(private readonly url: string) {
    this.label = url;
  }

  async load(): Promise<CanonicalGraph> {
    const res = await fetch(this.url);
    if (!res.ok) throw new Error(`fetch ${this.url}: ${res.status} ${res.statusText}`);
    return parseCanonicalGraph(await res.json(), this.url);
  }
}
