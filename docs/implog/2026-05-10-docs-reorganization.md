# Documentation Reorganization

**Date:** 2026-05-10

## Changes

### Deleted stale docs (all in git history)
- `docs/beads/` (14 files) — all beads completed, work captured in implog
- `docs/acute-loop-architecture.md` — spec fully implemented
- `docs/archive/` (7 files) — completed refactoring plans and historical dev notes
- `docs/research/pudl-mxk-*.md`, `pudl-yqt-*.md` — wrapper detection research, implemented
- `suggestions.md` — external review, items addressed
- 15 implog entries describing mu-extracted features (phases 1-8, glojure, vault, workflows)
- 6 implog entries about doc rewrites that were themselves rewritten

### Rescued
- `docs/archive/dev/inference_algorithm.md` → `docs/inference-algorithm.md` (live reference doc)

### Consolidated research
- `.ai/research/` → `docs/research/` (then pruned implemented items)

### Updated docs index
- `docs/README.md` updated to reflect current structure

## Net result
- 87 markdown files → ~45
- Remaining docs all describe current project state or forward-looking work

## Public API

No code changes. Documentation-only reorganization.
