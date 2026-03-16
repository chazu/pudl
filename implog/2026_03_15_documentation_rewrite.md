# Documentation Rewrite: Post-Extraction Cleanup

**Date:** 2026-03-15

## Summary

Rewrote three documentation files to reflect the current state of the codebase after the execution layer (models, methods, workflows, Glojure, executor, artifacts) was extracted into mu and cascade_priority was replaced with native CUE unification.

## Files Changed

### docs/cli-reference.md
- Removed all sections for: `pudl model`, `pudl method`, `pudl workflow`, `pudl process`, `pudl data search/latest` (as execution-layer commands)
- Retained `pudl data search` and `pudl data latest` since the cmd files still exist
- Added missing commands: `pudl setup`, `pudl completion`, `pudl module`, `pudl catalog`, `pudl verify`, `pudl export-actions`
- Updated all command documentation to match actual flags and behavior from cmd/*.go source
- Removed references to artifacts in `pudl list` flags

### docs/schema-authoring.md
- Removed all references to `cascade_priority`, `cascade_fallback`, and `compliance_level`
- Documented only the four valid `_pudl` metadata fields: `schema_type`, `resource_type`, `identity_fields`, `tracked_fields`
- Added a "How Validation Works" section explaining the two-phase process (heuristic scoring then CUE unification)
- Documented the `base_schema` inheritance chain as the fallback mechanism (no priority numbers)
- Updated all examples to remove `cascade_priority` from `_pudl` blocks

### docs/architecture.md
- Removed execution layer packages from the package table: model, executor, artifact, workflow, glojure, cue, effects
- Added current packages: mubridge, repo, skills, init, schema
- Removed storage layout entries for `.runs/` (workflow manifests) and `methods/` (Glojure files)
- Removed the "Two Execution Layers" section (CUE functions vs methods)
- Added a "Core Data Flow" diagram showing the actual system flow: import -> detect -> infer -> validate -> store
- Removed Glojure from the technology stack
- Updated all descriptions to reflect the current knowledge-layer-only architecture
