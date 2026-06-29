# Remove the pith `exec` cluster (Cluster B)

**Date:** 2026-06-26

## Summary

Removed the embedded pith VM query/scripting surface from pudl. It was a
May-2026 experiment (`pudl exec` + `internal/pithdriver`) that let a concatenative
pith program poke at the catalog/facts/schema via thin driver-word adapters.

It is fully superseded and removed **with no loss of functionality**:
- The real query layer is the Datalog engine (`internal/datalog`, surfaced by
  `pudl query` + `pkg/factstore.Query`, CUE-authored rules) — pith-free and
  strictly more capable than `catalog/query`'s flat filters.
- Every pith word was a one-line delegate to an existing `CatalogDB`/inference
  method already exposed by `pudl list`/`show`/`facts`/`facts add`/`schema list`
  and the `pkg/factstore` library.
- pith-as-execution now lives in **mu** (`mu/internal/pithvm`); the cass-memory
  loop runs on mu (`memory cycle`), not `pudl exec`.

See `docs/vestige-sweep.md` §2 (Cluster B) for the full analysis.

## Removed

- **Command:** `pudl exec` (`cmd/exec.go`) — ran a pith program against the lake.
- **Package:** `internal/pithdriver/` (8 files) — `catalog/*`, `fact/*`,
  `schema/*`, `drift/*` driver words + conversion helpers + examples test.
- **Dependency:** `github.com/chazu/pith` dropped from `go.mod` (require +
  `replace … => ../pith`); `go mod tidy` clears `go.sum`.
- **Docs (live surfaces only):** the `pith` topic + `printGuidePith` in
  `cmd/guide.go` (and its cross-references in the index/overview/datalog guides);
  the "Pith VM / `pudl exec`" section in `docs/cli-reference.md`.

Net: 12 files, **954 deletions**, −1 external dep, −1 replace directive.

## Public API change

- `pudl exec` is **gone**. No replacement needed — use `pudl query` /
  `pudl facts` / `pudl list` / `pudl show` / `pudl schema list`, or the
  `pkg/factstore` Go API, for the equivalent data operations.

## Deliberately left (not part of the code cluster)

Stale references that describe the removed command but are historical/design
artifacts, flagged for a later doc-cleanup decision rather than silently deleted:

- `implog/2026_05_11_pudl_exec_command.md`, `…_pithdriver_adapters.md`,
  `…_pith_arithmetic_and_drivers.md` — append-only work log (history; keep).
- `docs/pith-vm.md`, `docs/research/concatenative-vm.md`,
  `docs/chats/pith-jam-sesh.md` — design/research artifacts now orphaned.
- `docs/README.md`, `docs/cass-memory-substrate-plan.md`,
  `docs/mu-integration.md` — mention `pudl exec`; the mu-side pith references in
  these are still valid (mu has pith), the `pudl exec` mentions are stale.

## Verification

- `CGO_ENABLED=0 go build ./...` — clean.
- `go vet ./cmd/...` — clean.
- `CGO_ENABLED=0 go test ./...` — all pass.
- No residual `pithdriver` / `chazu/pith` / `execCmd` refs in Go.
