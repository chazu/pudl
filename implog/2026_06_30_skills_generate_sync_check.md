# 2026-06-30 — Embedded skills: go generate sync + drift check

The `pudl init` command writes AI-agent skill files from copies embedded in the
binary via `//go:embed files/*.md` (`internal/skills/skills.go`). Those embedded
copies are a hand-maintained duplicate of the canonical human-facing sources in
`skills/<name>/SKILL.md` (also symlinked from `.claude/skills/`). Nothing
enforced that the two stayed in sync, so the embedded copy — and therefore what
`pudl init` installs — could silently drift from the source of truth.

This adds a single-writer generator plus two guards so drift is impossible to
miss.

## What was added

### Generator — `internal/skills/gen/main.go` (`package main`)
Globs `skills/*/SKILL.md`, maps each to `internal/skills/files/<name>.md`, and
either writes the embedded copy (default) or verifies it (`-check`). Locates the
repo root by walking up to `go.mod`, so it works from any cwd (both the
`go:generate` contract dir `internal/skills` and the repo root).

- `go run ./internal/skills/gen` — sync (writes embedded copies)
- `go run ./internal/skills/gen -check` — verify; exits non-zero listing any
  out-of-sync skills, with a hint to run `go generate`.

### `//go:generate` directive — `internal/skills/skills.go`
Added `//go:generate go run ./gen` above the `//go:embed`, plus a comment
warning that `files/*.md` are generated and must not be edited by hand.

### Unit test — `TestEmbeddedSkillsInSync` (`internal/skills/skills_test.go`)
For every embedded skill, compares its bytes to `../../skills/<name>/SKILL.md`.
Runs with the normal `go test ./...`, so day-to-day drift is caught without a
separate CI step.

### Makefile
- `generate` → `go generate ./...`
- `check-skills` → `go run ./internal/skills/gen -check`
- `ci` now runs `check-skills` first (before `lint test-race coverage`).
- `.PHONY` updated.

## Verification
- In-sync: `make check-skills` passes, `make generate` produces no git diff,
  `TestEmbeddedSkillsInSync` passes.
- Drift injected into `files/pudl-core.md`: `make check-skills` fails (exit 1,
  names `pudl-core`) and the unit test fails; `make generate` restores sync.
- `go build ./...` and `go test ./internal/skills/` clean.

## Workflow going forward
Edit `skills/<name>/SKILL.md`, then `make generate` (or `go generate
./internal/skills`) and commit both the source and the regenerated
`internal/skills/files/<name>.md`. CI (`make ci`) and `go test ./...` both fail
if you forget.
