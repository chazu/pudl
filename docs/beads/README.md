# ACUTE Loop & Workspace Beads

Work items for closing the ACUTE convergence loop, adding per-repo workspaces, and making BRICK load-bearing.

## Dependency Graph

```
B1.1 Observe Ingestion ──┐
                          ├──> B1.3 Status Tracking ──> B1.4 Status CLI
B1.2 Manifest Ingestion ─┘

B2.1 Workspace Discovery ──┬──> B2.2 Repo Init Workspace
                           ├──> B2.3 Scoped Catalog Queries
                           ├──> B2.4 Workspace Definitions
                           └──> B2.5 Workspace Schema Resolution

B3.1 BRICK Toolchain (independent)
```

## Parallelization

These groups can run concurrently:

- **Group A:** B1.1 + B1.2 (no dependencies, can parallelize)
- **Group B:** B2.1 (must complete before B2.2-B2.5)
- **Group C:** B3.1 (fully independent)
- **Group D:** B2.2 + B2.3 + B2.4 + B2.5 (after B2.1, can parallelize with each other)
- **Group E:** B1.3 (after B1.1 + B1.2)
- **Group F:** B1.4 (after B1.3)

## Suggested Execution Order

1. **Wave 1** (parallel): B1.1, B1.2, B2.1, B3.1
2. **Wave 2** (parallel, after wave 1): B1.3, B2.2, B2.3, B2.4, B2.5
3. **Wave 3** (after B1.3): B1.4

## Epics

| Epic | Description | Beads |
|------|-------------|-------|
| [E1](E1-close-acute-loop.md) | Close the ACUTE feedback loop | B1.1, B1.2, B1.3, B1.4 |
| [E2](E2-per-repo-workspace.md) | Per-repo workspace support | B2.1, B2.2, B2.3, B2.4, B2.5 |
| [E3](E3-brick-toolchain.md) | BRICK-aware toolchain mapping | B3.1 |

## Beads

| ID | Title | Est. Lines | Blocked By |
|----|-------|-----------|------------|
| [B1.1](B1.1-observe-ingestion.md) | Observe result ingestion | ~200 | — |
| [B1.2](B1.2-manifest-ingestion.md) | Manifest ingestion | ~210 | — |
| [B1.3](B1.3-status-tracking.md) | Convergence status tracking | ~250 | B1.1, B1.2 |
| [B1.4](B1.4-status-command.md) | Status CLI command | ~120 | B1.3 |
| [B2.1](B2.1-workspace-discovery.md) | Workspace discovery & loading | ~180 | — |
| [B2.2](B2.2-repo-init-workspace.md) | Repo init generates workspace.cue | ~40 | B2.1 |
| [B2.3](B2.3-scoped-catalog.md) | Workspace-scoped catalog queries | ~80 | B2.1 |
| [B2.4](B2.4-workspace-definitions.md) | Workspace-aware definition discovery | ~80 | B2.1 |
| [B2.5](B2.5-workspace-schema-resolution.md) | Workspace-aware schema resolution | ~100 | B2.1 |
| [B3.1](B3.1-brick-toolchain.md) | BRICK-aware export-actions | ~50 | — |
