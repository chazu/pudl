# PUDL Documentation

This directory contains all PUDL documentation. Start with the [root README](../README.md) for a project overview.

## User Documentation

| Document | Description |
|----------|-------------|
| [getting-started.md](getting-started.md) | Installation, first import, first query |
| [concepts.md](concepts.md) | Core concepts: identity, schemas, inference, collections |
| [cli-reference.md](cli-reference.md) | All commands, flags, and examples |
| [schema-authoring.md](schema-authoring.md) | Writing custom CUE schemas with `_pudl` metadata |
| [collections.md](collections.md) | NDJSON, wrapper detection, collection queries |

## Architecture & Design

| Document | Description |
|----------|-------------|
| [architecture.md](architecture.md) | Streaming pipeline, catalog internals, storage layout, package structure |
| [VISION.md](VISION.md) | Project vision and roadmap |
| [plan.md](plan.md) | Development plan and roadmap |

## Testing

| Document | Description |
|----------|-------------|
| [TESTING.md](TESTING.md) | Test architecture, coverage, and benchmarks |

## Developer / Internal Docs

Implementation journals and design analysis documents are in [dev/](dev/):

| Document | Description |
|----------|-------------|
| [dev/inference_algorithm.md](dev/inference_algorithm.md) | Detailed inference algorithm with scoring tables and code |
| [dev/schema-inference-refactor.md](dev/schema-inference-refactor.md) | Schema inference refactor notes |
| [dev/schema_inference_plan.md](dev/schema_inference_plan.md) | Original inference implementation plan |
| [dev/schema_inference_divergence_analysis.md](dev/schema_inference_divergence_analysis.md) | Analysis of hard-coded vs CUE-based detection |
| [dev/collection_review.md](dev/collection_review.md) | Collection handling design review |
| [dev/cue-loader-debug-fix.md](dev/cue-loader-debug-fix.md) | CUE loader debugging notes |
| [dev/implementation_notes.md](dev/implementation_notes.md) | Phase 1-2 implementation journal |
| [dev/implementation_log_2025_08_25.md](dev/implementation_log_2025_08_25.md) | August 25 implementation log |
| [dev/implementation_log_2025_08_29.md](dev/implementation_log_2025_08_29.md) | August 29 implementation log |
