# ACUTE Loop & Workspace Architecture Planning

## Date
2026-03-24

## Summary

Created architecture documentation and detailed implementation beads for three epics:

1. **Close the ACUTE Feedback Loop** (E1) — observe ingestion, manifest ingestion, convergence status tracking, status CLI
2. **Per-Repo Workspace Support** (E2) — workspace discovery, repo init workspace.cue, scoped catalog, workspace-aware definitions and schemas
3. **BRICK-Aware Toolchain Mapping** (E3) — extract toolchain from BRICK #Target metadata

## Files Created

- `docs/acute-loop-architecture.md` — full architecture document covering all 6 areas of work
- `docs/beads/README.md` — bead index with dependency graph and parallelization strategy
- `docs/beads/E1-close-acute-loop.md` — epic 1 definition
- `docs/beads/E2-per-repo-workspace.md` — epic 2 definition
- `docs/beads/E3-brick-toolchain.md` — epic 3 definition
- `docs/beads/B1.1-observe-ingestion.md` — observe result ingestion bead
- `docs/beads/B1.2-manifest-ingestion.md` — manifest ingestion bead
- `docs/beads/B1.3-status-tracking.md` — convergence status tracking bead
- `docs/beads/B1.4-status-command.md` — status CLI command bead
- `docs/beads/B2.1-workspace-discovery.md` — workspace discovery bead
- `docs/beads/B2.2-repo-init-workspace.md` — repo init workspace.cue bead
- `docs/beads/B2.3-scoped-catalog.md` — scoped catalog queries bead
- `docs/beads/B2.4-workspace-definitions.md` — workspace-aware definitions bead
- `docs/beads/B2.5-workspace-schema-resolution.md` — workspace-aware schema resolution bead
- `docs/beads/B3.1-brick-toolchain.md` — BRICK-aware export-actions bead

## Files Modified

- `docs/plan.md` — added Active section with epic/bead checklist, added workspace package to core packages table
