# PUDL Documentation

Start with the [root README](../README.md) for a project overview.

## Contents

| Document | Description |
|----------|-------------|
| [evidence.md](evidence.md) | Exact evidence, report baselines and witnesses, retention, exports, classification traces, and portable backups |
| [getting-started.md](getting-started.md) | Install PUDL, create a repository, and find Git inventory drift using PUDL commands |
| [concepts.md](concepts.md) | Core concepts: identity, schemas, inference, collections, and value wiring |
| [cli-reference.md](cli-reference.md) | All commands, flags, and examples |
| [schema-authoring.md](schema-authoring.md) | Writing custom CUE schemas with `_pudl` metadata |
| [collections.md](collections.md) | NDJSON collections, typed envelopes, membership, and queries |
| [workspace.md](workspace.md) | Self-contained repository state, local schema resolution, and global fallback |
| [architecture.md](architecture.md) | Streaming pipeline, catalog internals, storage layout, package structure |
| [architecture-improvement-report.md](architecture-improvement-report.md) | Highest-leverage architecture improvements and design questions |
| [UX simplification design report](design/2026-09-30-ux-simplification-report.md) | Project evolution, proposed scope reduction, and simpler workflows for humans and agents |
| [TESTING.md](TESTING.md) | Test architecture, coverage, and benchmarks |
| [facts.md](facts.md) | Bitemporal fact store: schema, temporal queries, CLI commands |
| [datalog.md](datalog.md) | Datalog evaluator: writing rules, `pudl query`, EDB sources, `catalog_entry` relation, performance |
| [library-api.md](library-api.md) | Public Go API (`pkg/factstore`, `pkg/eval`) for external programs |
| [VISION.md](VISION.md) | Project vision and roadmap |
| [mu-integration.md](mu-integration.md) | pudl ↔ mu collaboration: drift convergence and data import |
| [mu-pudl-batcave-user-experience.md](mu-pudl-batcave-user-experience.md) | Operator feedback from configuring and testing the three-board Batcave fleet |
| [cross-model-dependencies.md](cross-model-dependencies.md) | Exact model sets, dependency facts, and current value-wiring contract |
| [inference-algorithm.md](inference-algorithm.md) | Schema inference engine: heuristics, CUE unification, scoring |
| [plan.md](plan.md) | Living development plan: what's built, what's next |

## Subdirectories

| Directory | Description |
|-----------|-------------|
| [design/](design/) | Design reports and contracts; each document records its proposal or delivery status |
| [research/](research/) | Design proposals and research notes |
| [issues/](issues/) | Open issues and known gaps |
| [../implog/](../implog/) | Implementation logs (chronological, top-level directory) |
