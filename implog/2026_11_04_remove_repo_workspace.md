# Remove the repository's own pudl workspace

**Date:** 2026-11-04

## Work done

The source repository no longer carries a pudl workspace. `.pudl/` (61
tracked files: built-in schemas, an installed copy of the GCP pack, its
fixtures and install record, tracked since 7fe270d) is untracked and ignored
(`/.pudl/` in `.gitignore`). With no `.pudl/workspace.cue` in the tree, pudl
run inside the repository uses the global `~/.pudl`.

- The local copy was moved to `~/.pudl-repo-backup/pudl-repo-2026-11-04`.
  Its catalog held only fixture runs (42 entries), so nothing was migrated
  to the global catalog.
- Smoke tests still use `.pudl/data/` as scratch space and recreate it; it
  is now ignored as a whole.
- Why: the tracked copy of `examples/gcp` drifted whenever the pack changed,
  and installing a pack from inside the repository changed tracked files.

## Public API

None.
