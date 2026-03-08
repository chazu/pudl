# Documentation Consistency Update — 2026-03-08

## Summary

Brought all user-facing documentation up to date with the complete infrastructure automation expansion (Phases 1-8). Fixed incorrect information, added missing commands, and created new guides.

## Files Modified

| File | Changes |
|------|---------|
| `README.md` | Updated description, added automation commands, updated project status (352 tests) |
| `docs/VISION.md` | Moved Phases 1-8 to "What Exists Today", updated CLI commands list, added Glojure/age to tech stack |
| `docs/getting-started.md` | Added sections: Define a Model, Create a Definition, Run a Method, Run a Workflow, Check for Drift, Manage Secrets |
| `docs/cli-reference.md` | Added: method run/list, workflow run/list/show/validate/history, drift check/report, vault get/set/list/rotate-key, data search/latest, model search/scaffold, list --artifacts/--all |
| `docs/concepts.md` | Added: Methods & Execution, Artifacts, Workflows, Drift Detection, Vault, Effects sections. Updated workspace layout |
| `docs/architecture.md` | Added: models/, methods/, definitions/, extensions/, .runs/, .drift/, vaults/ to storage layout. Added artifact catalog columns. Added 10 new packages to table. Added Glojure to tech stack |
| `docs/model-authoring.md` | Added: model search, model scaffold, extension models, effect pattern. Removed "Phase 2" reference |
| `docs/definition-authoring.md` | Fixed vault syntax from `vault."path"` to `vault://path`. Removed "Phase 5 Preview" and "not yet implemented". Added vault CLI examples |
| `docs/README.md` | Reorganized into Data Pipeline, Infrastructure Automation, Reference, Architecture sections. Added links to new docs |

## Files Created

| File | Purpose |
|------|---------|
| `docs/method-authoring.md` | Method writing guide: file convention, run function, method kinds, builtins, effects, lifecycle |
| `docs/workflows.md` | Workflow composition guide: CUE format, step fields, dependency resolution, concurrent execution, manifests |
| `docs/drift.md` | Drift detection guide: check/report commands, flags, report storage |
| `docs/vault.md` | Vault guide: references, backends (env/file), configuration, CLI commands, security |

## Verification

- `grep -r "not yet implemented" docs/` returns only the analytics Future Vision section (correct)
- `grep -r "Phase.*Preview" docs/` returns nothing
- All cross-links between docs resolve to existing files
- All CLI commands from `cmd/*.go` have corresponding entries in `docs/cli-reference.md`
