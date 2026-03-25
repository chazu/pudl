# Epic 2: Per-Repo Workspace Support

## Goal

Make pudl definitions and schemas portable across repositories by supporting a per-repo `.pudl/` directory with `workspace.cue` as the marker file. The global `~/.pudl/` catalog remains the single database, scoped by workspace origin.

## Beads

- B2.1 — Workspace discovery and loading
- B2.2 — Extend `pudl repo init` to generate workspace.cue
- B2.3 — Workspace-scoped catalog queries
- B2.4 — Workspace-aware definition discovery
- B2.5 — Workspace-aware schema resolution

## Dependencies

Can be done independently of Epic 1, but the full ACUTE loop benefits from both.

## Acceptance Criteria

```bash
cd my-project
pudl repo init                    # creates .pudl/workspace.cue
# add definitions to .pudl/definitions/
pudl definition list              # finds per-repo definitions
pudl list                         # shows only this workspace's entries
pudl list --all-workspaces        # shows everything
pudl drift check --all            # uses per-repo definitions
pudl export-actions --all         # uses per-repo toolchain mappings
```
