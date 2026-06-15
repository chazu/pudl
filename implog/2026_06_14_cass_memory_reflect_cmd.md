# cass-memory — configurable reflect agent command

Date: 2026-06-14

## Summary

Make the reflect step's agent configurable. Reflect needs a *tool-calling coding
agent* (it works by having the agent run `pudl facts observe` itself), not a raw
completion API — so configurability is about swapping the agent command
(claude / aider / cursor-agent / custom), not wiring an HTTP LLM. No pith
`llm/*` word is involved.

## Changes

### `internal/config/config.go`
- `Config.ReflectCommand` (`reflect_command` YAML key) — the agent command the
  reflect target invokes; empty means use the default.

### `cmd/memory.go`
- `memoryCycleTemplate` const → `memoryCycleWorkspace(reflectCmd string)`: the
  reflect target's command is built from `reflectCmd` with the prompt appended as a
  quoted argument.
- `pudl memory init --reflect-cmd "<cmd>"`. Precedence: flag > `reflect_command`
  config key > default `"claude -p"`. `init` reports the chosen command.

## Verification

- CLI: default → `claude -p`; `--reflect-cmd "aider --message"` → that command;
  `reflect_command` config key used when no flag (precedence correct).
- `mu build --config <generated> --plan //memory:cycle` plans
  `sh -c aider --message 'Review ...'` — valid CUE, mu accepts it.
- `CGO_ENABLED=0 go test ./...` full suite green.

## Notes

- The prompt is appended as a trailing quoted argument, which fits agents like
  `claude -p` and `aider --message`. An agent that takes the prompt differently
  (stdin, a different flag) is handled by editing the generated mu.cue — the user
  owns it.
- A pith `llm/complete` word remains deferred and is only relevant for a future
  raw-completion step (e.g. a yes/no validate gate), never for reflect.
