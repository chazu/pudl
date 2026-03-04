# Documentation Restructure

**Date:** 2026-03-03

## Summary

Restructured PUDL documentation from a single monolithic `docs/README.md` into a focused, multi-document structure with a root README. Key improvements:

1. **Added root `README.md`** — Project overview, quick start, and links to detailed docs
2. **Created `docs/concepts.md`** — Explains the mental model: content-addressed IDs, proquint encoding, three layers of identity, CUE schemas with `_pudl` metadata, schema inference algorithm, cascade validation, and collections
3. **Created `docs/getting-started.md`** — Install, first import, first query, collections, custom schemas
4. **Created `docs/cli-reference.md`** — Complete command reference with all flags and examples
5. **Created `docs/schema-authoring.md`** — Guide to writing custom CUE schemas with `_pudl` metadata blocks, including all metadata fields, inheritance, examples, and tips
6. **Created `docs/collections.md`** — NDJSON, wrapper detection scoring algorithm, collection structure, querying, deletion
7. **Created `docs/architecture.md`** — Storage layout, catalog schema, streaming pipeline, import flow, package structure, tech stack
8. **Reorganized `docs/README.md`** — Now serves as documentation index linking to all docs
9. **Moved internal docs to `docs/dev/`** — Implementation journals, design analyses, and algorithm details separated from user-facing docs
10. **Cleaned up `docs/plan.md`** — Separated completed work log (now a summary table pointing to `implog/`) from the active roadmap

## Files Created

- `README.md` — Root project README
- `docs/concepts.md` — Core concepts guide
- `docs/getting-started.md` — Getting started guide
- `docs/cli-reference.md` — CLI reference
- `docs/schema-authoring.md` — Schema authoring guide
- `docs/collections.md` — Collections guide
- `docs/architecture.md` — Architecture documentation
- `implog/2026_03_03_documentation_restructure.md` — This file

## Files Modified

- `docs/README.md` — Rewritten as documentation index
- `docs/plan.md` — Trimmed to roadmap + summary of completed work

## Files Moved to `docs/dev/`

- `docs/inference_algorithm.md`
- `docs/schema-inference-refactor.md`
- `docs/schema_inference_divergence_analysis.md`
- `docs/schema_inference_plan.md`
- `docs/collection_review.md`
- `docs/cue-loader-debug-fix.md`
- `docs/implementation_notes.md`
- `docs/implementation_log_2025_08_25.md`
- `docs/implementation_log_2025_08_29.md`
