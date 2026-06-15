# cass-memory substrate — Orchestration (memory loop + hooks)

Date: 2026-06-14

## Summary

The orchestration that turns the substrate into a running ACE loop (plan §8/§9):
the Generator command, the mu cycle scaffold + runner, and the harness hook
installer. The invariant holds — pudl stores and scores; the reasoning runs as mu
pith targets that shell back into this CLI, and the LLM step is agent-native
(`claude -p`), so no API key and no pith `llm/*` word are needed for v1.

Two simplifications fell out during implementation:

1. **No mu `-C` flag needed.** mu already has `--config <mu.cue>`, which roots the
   build at that file's directory (`resolveProjectRoot` → `filepath.Dir`). So
   `mu build --config ~/.pudl/mu.cue //memory:cycle` gives explicit rooting with
   zero mu changes. (Validated: `mu build --config <generated> --plan //memory:cycle`
   plans all three actions, rooted at `.pudl`.)
2. **No pith `llm/*` word needed.** The reflect target uses `exec/shell` →
   `claude -p`, reusing the existing agent.

## Changes

### `internal/database/memory_context.go` (new)
- `MemoryContext(task, limit)`: ranked promoted observations. With `task`, FTS5
  keyword match joined to `fact_scored` and ordered by `decayed_worth`; without,
  top `decayed_worth`. Read-only — the Generator, no model calls.

### `cmd/memory.go` (new) — `pudl memory`
- `context`: prints ranked promoted observations as a markdown block for context
  injection (`--task`, `--limit`, `--json`); silent when empty (hook-friendly).
- `init`: writes `<pudl dir>/mu.cue` — a valid 3-target cycle (reflect → curate →
  cycle). reflect shells `claude -p` (agent-native); curate runs `pudl facts
  curate`. `--force` to overwrite.
- `cycle`: runs `mu build --config <pudl dir>/mu.cue --no-cache //memory:cycle`;
  guards on missing mu / missing workspace.

### `cmd/hooks.go` (new) — `pudl hooks`
- `print`: emits the Claude Code settings.json hook snippet (non-destructive).
- `install [--scope user|project] [--dry-run]`: idempotent merge into settings.json
  with a `.bak` backup; preserves unrelated settings and existing hooks.
- Managed hooks: `SessionStart → pudl memory context`, `Stop → pudl facts curate`.

### `cmd/prime.go`
- Documented the self-improvement loop commands.

## Safety decision — Stop runs curate, not the reflect cycle

The Stop hook runs only `pudl facts curate` (deterministic, cheap, no LLM), NOT
`pudl memory cycle`. reflect shells out to `claude -p`; running that on every Stop
risks recursion (a headless agent whose own Stop fires the hook again) and cost.
The full reflect cycle is meant to be triggered from a scheduler (cron / scheduled
agent), which is out of scope here.

## Verification

- `cmd/hooks_test.go`: merge into empty (2 hooks, idempotent on repeat); merge
  preserves an unrelated `model` setting and an existing Stop hook while adding ours.
- CLI e2e: `memory context` (promoted-only, ranked; `--task` keyword filter);
  `memory init` writes valid CUE (3 targets, confirmed via cuecontext and
  `mu --plan`); `memory cycle` guards on missing mu; `hooks print`/`install`
  (idempotent, merge + backup).
- `mu build --config <generated mu.cue> --plan //memory:cycle` plans reflect →
  curate → cycle, rooted at the pudl dir.
- `CGO_ENABLED=0 go test ./...` full suite green.

## Status

This completes the cass-memory rebuild: substrate (Phases A–D + comparison
operators) + Curator + orchestration. The whole loop is runnable —
`pudl memory init`, `pudl hooks install`, and a scheduled `pudl memory cycle`.

Deferred (as designed): pith `llm/complete` word (Option B, external-key path);
dlktk conflict resolution; per-harness hook adapters beyond Claude Code; the mu
"home workspace" global-target feature (plan §10).
