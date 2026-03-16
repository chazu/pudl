# Documentation Rewrite: concepts.md and getting-started.md

**Date:** 2026-03-15

## Summary

Rewrote `docs/concepts.md` and `docs/getting-started.md` to reflect the current state of the codebase after the execution layer removal and cascade-to-CUE-unification migration.

## What Changed

### concepts.md

Removed all sections covering:
- Models (schema vs model vs definition table with model concept)
- Methods and execution (Glojure, lifecycle dispatch, method kinds)
- Artifacts (method outputs, run IDs, tags)
- Workflows (DAG orchestration)
- Vault (credential management)
- Cascade priority / compliance levels in `_pudl` metadata
- Effects (dry-run effect descriptions)
- Three validation layers table (base schema / policy schemas / qualification methods)

Updated or added sections for:
- Data Import Pipeline (format detection, collection handling, streaming)
- Content-Addressed Identity (SHA256, proquint, dedup, three layers)
- Schema System (`_pudl` metadata with only schema_type, resource_type, identity_fields, tracked_fields)
- Schema Inference (heuristic scoring + CUE unification)
- Validation (native CUE unification with intended -> base -> catchall fallback, never-reject philosophy)
- Definitions (CUE values conforming to schemas, socket wiring, dependency graph)
- Drift Detection (declared vs imported state, mu bridge export)
- Fixed-Point Verification (idempotency check via `pudl verify`)
- Catalog Layer (central registry of known schema types)
- Doctor (workspace health checks)
- Workspace Layout (updated to remove models/, methods/, extensions/, vaults/, .runs/)

### getting-started.md

Removed all sections covering:
- Define a Model (model list, search, scaffold, show)
- Run a Method (method run, dry-run, method list)
- Run a Workflow (workflow run, validate, show, history)
- Manage Secrets (vault set, get, list)
- Drift refresh flag
- References to model-authoring.md, method-authoring.md, workflows.md, drift.md, vault.md

Restructured as a numbered tutorial:
1. Install and initialize
2. Import some data
3. List and show entries (including catalog command)
4. Write a schema (with inline CUE example showing current _pudl metadata)
5. Validate data against schemas
6. Create a definition
7. Check for drift (including export-actions)
8. Run verification

Kept useful reference sections: delete, config, doctor, large files.

## Public API

No code changes -- documentation only.
