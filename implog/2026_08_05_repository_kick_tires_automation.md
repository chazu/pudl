# Repository kick-the-tires automation

**Date:** 2026-08-05

## Outcome

The formerly manual repository-local run-set matrix is now a checked-in smoke
contract. `make test-kick-tires` builds the current PUDL binary beneath
`.pudl/data/smoke/bin/`, creates isolated nested repositories beneath
`.pudl/data/kick-tires/test-runs/`, installs the tracked CUE/plugin fixtures, and
drives the public CLI through real mu. An independent CI job installs mu v0.3.3
and runs the target on every push and pull request.

The matrix covers plain producer/consumer ordering and reuse, fail-fast graph
and template validation, projection authorization, failed-producer propagation,
durable approval/resume/reject, stale plans, single-model sealed provider I/O,
the known fail-closed sealed run-set boundary, write-policy denial, and two
simultaneous run-sets. Existing Go tests continue to cover crash recovery and
approval races in the normal `go test ./...` suite.

## Bugs found by automation

- Concurrent first opens of an empty catalog raced while creating the migration
  ledger. Catalog schema initialization now uses a short cross-process advisory
  lock; ordinary reads and writes remain governed by SQLite/WAL.
- PUDL generated non-hidden subprojects beneath a shared mu root. Concurrent
  runs therefore became visible to each other through mu's recursive config
  merge, producing duplicate target names or deleting a live workspace. A
  per-mu-root advisory lock now covers each generated workspace and its mu calls.

Both fixes have fast regressions: concurrent `NewCatalogDB` first-open stress in
`internal/database`, and the original two-process real-CLI smoke test.

## Public verification

```bash
make test-kick-tires
mise exec -- go test ./...
mise exec -- env CGO_ENABLED=0 go test ./...
```
