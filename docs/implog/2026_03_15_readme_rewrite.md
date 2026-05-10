# README Rewrite -- 2026-03-15

## Summary

Rewrote `/README.md` and `/docs/README.md` to reflect the current state of pudl after the execution layer removal refactoring.

## Changes

### README.md
- Removed all references to models, methods, workflows, Glojure runtime, executor, artifact store, CUE processor, effects, agent integration
- Removed commands: `pudl model/*`, `pudl method/*`, `pudl workflow/*`, `pudl data search/latest`
- Removed concepts: cascade_priority, cascade_fallback, compliance_level, ComplianceStatus, CascadeLevel
- Updated project description to focus on data lake, schema inference, and drift detection
- Added mu integration section explaining the export-actions bridge
- Updated command tables to match current CLI surface
- Updated project status to reflect the refactoring

### docs/README.md
- Simplified to a single table of contents
- Removed references to removed docs: model-authoring.md, definition-authoring.md, method-authoring.md, workflows.md, drift.md, vault.md
- Removed dev/ section (internal docs not part of user-facing documentation index)
- Kept references to: concepts.md, getting-started.md, cli-reference.md, schema-authoring.md, collections.md, architecture.md, TESTING.md, VISION.md
