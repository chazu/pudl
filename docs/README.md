# PUDL Documentation

Start with the [root README](../README.md) for a project overview.

## Contents

| Document | Description |
|----------|-------------|
| [getting-started.md](getting-started.md) | Installation, first import, models, and exact run-sets |
| [concepts.md](concepts.md) | Core concepts: identity, schemas, inference, collections, and value wiring |
| [cli-reference.md](cli-reference.md) | All commands, flags, and examples |
| [schema-authoring.md](schema-authoring.md) | Writing custom CUE schemas with `_pudl` metadata |
| [collections.md](collections.md) | NDJSON collections, typed envelopes, membership, and queries |
| [workspace.md](workspace.md) | Self-contained repository state, local schema resolution, and global fallback |
| [architecture.md](architecture.md) | Streaming pipeline, catalog internals, storage layout, package structure |
| [architecture-improvement-report.md](architecture-improvement-report.md) | Highest-leverage architecture improvements and design questions |
| [TESTING.md](TESTING.md) | Test architecture, coverage, and benchmarks |
| [facts.md](facts.md) | Bitemporal fact store: schema, temporal queries, CLI commands |
| [datalog.md](datalog.md) | Datalog evaluator: writing rules, `pudl query`, EDB sources, `catalog_entry` relation, performance |
| [library-api.md](library-api.md) | Public Go API (`pkg/factstore`, `pkg/eval`) for external programs |
| [VISION.md](VISION.md) | Project vision and roadmap |
| [mu-integration.md](mu-integration.md) | pudl ↔ mu collaboration: drift convergence and data import |
| [cross-model-dependencies.md](cross-model-dependencies.md) | Exact model sets, dependency facts, and current value-wiring contract |
| [inference-algorithm.md](inference-algorithm.md) | Schema inference engine: heuristics, CUE unification, scoring |
| [plan.md](plan.md) | Living development plan: what's built, what's next |

## Subdirectories

| Directory | Description |
|-----------|-------------|
| [research/](research/) | Design proposals and research notes |
| [issues/](issues/) | Open issues and known gaps |
| [implog/](implog/) | Implementation logs (chronological) |
