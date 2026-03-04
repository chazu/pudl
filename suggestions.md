# PUDL Project Review

## What This Is

PUDL is a personal data lake CLI for cloud infrastructure data. You import JSON/YAML/CSV/NDJSON, it catalogs entries in SQLite, validates against CUE schemas, and lets you list/query/export. The aspiration is outlier detection and infrastructure sprawl reduction.

**Current state**: Solid import-catalog-query pipeline. Well-tested (all green, 291 tests). Clean Go code. But it's a data storage tool that doesn't yet do anything *analytical* with the data it stores.

---

## The Core Problem

PUDL's identity is split between two things:

1. **A data cataloging tool** (what exists) — import data, assign schemas, list/query entries
2. **An infrastructure analysis tool** (the vision) — outlier detection, correlation, sprawl reduction

Everything built so far serves (1). Nothing serves (2). The tool can store and retrieve cloud data but can't tell you anything *about* it that you didn't already know. This is the gap that matters.

---

## What's Working Well

- **Import pipeline** — Format detection, multi-format support, collection handling. This is solid.
- **SQLite catalog** — Good query interface with filtering/sorting/pagination. Performant.
- **CUE schema inference** — Automatic schema detection with heuristics + CUE unification. Clever and functional.
- **Schema management** — `schema new`, `schema reinfer`, `schema migrate`, git integration. The schema lifecycle is well thought out.
- **Schema name normalization** — Recent work, clean package with good tests.
- **Export** — Multi-format output works.
- **Test suite** — Good coverage of the core packages, integration tests, system tests.
- **Doctor** — Useful health check utility.

---

## What's Underdeveloped

**1. The entire analytical layer doesn't exist.**
The VISION says "outlier detection and sprawl reduction" is the *primary goal*. There's no implementation of:
- Outlier detection
- Cross-source correlation
- Policy compliance (the two-tier schema concept)
- Any kind of aggregation, comparison, or reporting
- No `pudl analyze`, `pudl report`, `pudl diff`, or similar

Without this, pudl is a worse `jq` + `sqlite3` combo. The analytical layer is what would make it worth using.

**2. The schema review workflow (`internal/review/`)** — 6 files, session management, review states, but:
- No tests for any of it
- The interactive reviewer uses raw `fmt.Scanf` prompts, not the Bubble Tea TUI claimed in the VISION
- It's unclear if this has ever been run end-to-end

**3. The query/list experience** — `pudl list` works but there's no way to ask interesting questions like "show me all EC2 instances that differ from the majority" or "which resources changed between two imports." Listing catalog entries is table stakes, not the value prop.

---

## What's Superfluous (Cut Candidates)

**1. `op/` + `internal/cue/processor.go` + `cmd/process.go`** — A CUE custom function processor that does Uppercase/Lowercase/Concat. This is an early experiment completely unrelated to pudl's purpose. **Cut it.**

**2. `cmd/setup.go`** (484 lines) — Shell integration that installs `pcd` alias and `pudl-cd` function. This is premature convenience optimization for a tool that hasn't proven its core value yet. A single line in the README ("add `alias pcd='cd ~/.pudl/schema'` to your shell config") replaces this. **Cut it.**

**3. `cmd/module.go`** (251 lines) — Thin wrapper around `cue mod tidy`, `cue mod get`, `cue mod edit`. Doesn't add meaningful value over telling users to `cd ~/.pudl/schema && cue mod tidy`. **Cut it.**

**4. `cmd/git.go`** (135 lines) — Only has a `cd` subcommand. The `git` namespace implies you can do git operations (commit, push, diff), but you can't — that's on `schema commit/status/log`. This is misleading and redundant with `setup.go`'s `pcd` alias. **Cut it.**

**5. `internal/streaming/`** (4100+ lines, 12 files) — CDC-based streaming parser with memory monitoring, backpressure, and progress reporting. This is a future need, not a current one. That's 4100 lines of speculative complexity. The standard import path handles everything needed today. **Mothball it** — move to a branch, cut from main, bring it back when actually needed.

**6. Bubble Tea dependency** — Imported and used only for a list model in `internal/ui/`. Not used for any interactive TUI workflow. Either commit to building TUI interactions or drop the dependency and use plain text output (which is what's mostly happening anyway). Given the review workflow uses `fmt.Scanf`, the TUI ambition isn't being realized. **Drop it for now.**

---

## Structural Issues

**1. `cmd/schema.go` is 1901 lines.** This is the worst offender against the project's own CLAUDE.md rule of 300-500 lines max. It contains ~10 subcommands, the full review workflow, schema reinfer logic, and migration code. This file needs to be split into at least 4-5 files.

**2. The root command description is wrong.** It says "Automatic schema inference using embedded Lisp rules." Zygomys/Lisp was abandoned. This is user-facing text that's inaccurate.

**3. VISION.md is misleading.** It describes Zygomys, DuckDB, Parquet, Bubble Tea TUI, two-tier schemas, cross-source correlation — none of which exist. Anyone reading this doc would have a completely wrong picture of the tool. It should be updated to reflect reality and clearly separate "what exists" from "what's aspirational."

**4. `docs/plan.md` is stale.** It still lists "Complete CUE-based schema detection" as high priority when recent commits show that work has progressed significantly. The plan should reflect current state.

---

## The Focused Path Forward

### Phase 1: Clean House (1-2 sessions)
- Cut dead code (`op/`, `process`, `setup`, `module`, `git cd`)
- Mothball streaming to a branch
- Split `cmd/schema.go` into focused files
- Fix root command description
- Update VISION.md and plan.md to reflect reality

### Phase 2: Make Import -> Query Actually Useful (the critical path)
- Add `pudl diff` — compare two imports of the same resource type, show what changed
- Add `pudl summary` or `pudl stats` — aggregate view ("you have 47 EC2 instances, 3 don't match the common pattern")
- Add basic outlier detection — given N instances of a schema, which ones have unusual field values?
- These three features are what turn pudl from "a place data goes" into "a tool that tells me things"

### Phase 3: Deepen Schema Intelligence
- Implement the two-tier schema (type recognition + policy compliance)
- Schema drift detection — "this resource used to validate, now it doesn't"
- Schema coverage report — "37% of your data matches a specific schema, 63% is generic"

### Phase 4: Correlation & Cross-Source
- Link AWS resources to K8s resources
- Temporal tracking — same resource across multiple imports
- This is where the real power lives, but it needs the analytical foundation from Phase 2/3

---

## Summary

PUDL has strong engineering fundamentals — clean Go, good tests, solid import pipeline, smart schema inference. But it's infrastructure without a payoff. The tool can store data but can't tell you anything useful about it yet. The codebase also carries ~5000 lines of dead/speculative code that dilutes focus.

The single most impactful thing you could do is build the analytical layer (diff, summary, outlier detection). That's what turns "a personal data catalog" into "a tool that amplifies my ability to understand infrastructure" — which is what the README promises.
