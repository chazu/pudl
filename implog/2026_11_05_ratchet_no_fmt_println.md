# Ratchet: no fmt.Println outside cmd/ (2026-11-05)

## What changed

- `ratchets/ast/no-fmt-println-outside-cmd.toml` — tree-sitter (AST) rule
  matching `fmt.Println(...)` calls in Go. AST matching ignores the text in
  comments and strings. Excludes `cmd/**` and `**/*_test.go` (Go Example
  functions need `fmt.Println` for their checked output).
- `ratchets.toml` — rule added to `enabled_ratchets`.
- `ratchet-counts.toml` — budget `"." = 1`.

## Current violations (1)

- `internal/ui/ui.go:14` — `RunInteractiveList` prints "No data found..."
  when there are no entries.

Removed 2026-11-05 (`pudl-4l1`): `cmd/list.go` already prints the message before calling `RunInteractiveList`. Budget is now 0.
