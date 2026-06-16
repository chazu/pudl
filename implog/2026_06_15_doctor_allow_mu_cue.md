# Doctor: allow `mu.cue` at workspace top level

## Problem

`pudl doctor` flagged `~/.pudl/mu.cue` as an unexpected top-level entry:

```
⚠️  Directory Structure
   Status: warning
   Message: Unexpected entries in PUDL workspace
   Details: Unexpected top-level entries in /Users/chazu/.pudl: [mu.cue]
```

`mu.cue` is a legitimate file written by `pudl memory init` — it's the memory
cycle config (`mu build --config ~/.pudl/mu.cue //memory:cycle`). The
`CheckDirectoryStructure` exhaustive-validation allowlist did not include it.

## Fix

Added `"mu.cue"` to the `allowedTopLevel` set in
`CheckDirectoryStructure` (`internal/doctor/checks.go`).

## Public API

No API change. Behavioral: `pudl doctor` no longer warns about the presence of
the memory-cycle config at `~/.pudl/mu.cue`.

## Verification

- `pudl doctor` → Directory Structure now `ok`.
- `CGO_ENABLED=0 go test ./internal/doctor/` passes.
