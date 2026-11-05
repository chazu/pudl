// Why the graph shows what it shows: how many catalog entries became
// resources, which schemas' entries did not (and why), and whether any
// relationship rules are loaded. A sparse graph should explain itself.

import type { Coverage } from '../model/canonical';

const REASONS: Record<string, string> = {
  'no-identity':
    'No resource identity: these records matched no type with identity fields (often a generic catch-all type). If a pack now covers them, `pudl doctor --fix` reclassifies the records already imported. Otherwise install a pack or write a type; `pudl import --dry-run --explain` shows how a record is classified.',
  internal: "pudl's own records (check models, observations); not infrastructure.",
};

export function CoveragePanel({ coverage }: { coverage: Coverage }) {
  const hidden = coverage.hidden ?? [];
  const hiddenEntries = hidden.filter((h) => h.reason !== 'internal').reduce((n, h) => n + h.entries, 0);
  return (
    <section className="coverage">
      {coverage.edgeRules === 0 && coverage.resources > 0 && (
        <div className="notice warn-notice">
          No relationship rules are loaded, so nothing is connected. Packs installed before graph support lack them:
          re-run <code>pudl pack install gcp</code> (or <code>aws</code>), adding <code>--overwrite</code> if it
          reports conflicts, then <code>pudl doctor --fix</code> to reclassify records with the new types.
        </div>
      )}
      <details open={hiddenEntries > coverage.resources}>
        <summary>
          {coverage.resources} resources shown from {coverage.entries} catalog entries
          {hiddenEntries > 0 && <span className="warn"> · {hiddenEntries} not shown</span>}
        </summary>
        {hidden.length === 0 ? (
          <div className="muted">Every catalog entry is in the graph.</div>
        ) : (
          <ul className="hidden-schemas">
            {hidden.map((h) => (
              <li key={`${h.schema}:${h.reason}`} title={REASONS[h.reason] ?? h.reason}>
                <code>{h.schema}</code> <span className="count">{h.entries}</span>
                <div className="muted">{REASONS[h.reason] ?? h.reason}</div>
              </li>
            ))}
          </ul>
        )}
      </details>
    </section>
  );
}
