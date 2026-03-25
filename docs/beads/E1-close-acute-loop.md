# Epic 1: Close the ACUTE Feedback Loop

## Goal

Make pudl's convergence loop work end-to-end by adding the return path: mu results flow back into pudl state, and per-resource convergence status is queryable.

## Beads

- B1.1 — Observe result ingestion
- B1.2 — Manifest ingestion
- B1.3 — Convergence status tracking
- B1.4 — Status CLI command

## Dependencies

None — this epic uses the existing global `~/.pudl/` workspace.

## Acceptance Criteria

After this epic, the following pipeline works without manual intervention between steps:

```bash
pudl drift check --all
pudl export-actions --all > mu.json
mu build --emit-manifest > manifest.json
pudl ingest-manifest --path manifest.json
mu observe --json | pudl ingest-observe
pudl status  # shows converged/failed/clean per definition
pudl drift check --all  # uses observe results as live state
```
