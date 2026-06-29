# Confirmed-dead quick wins (vestige sweep §1)

**Date:** 2026-06-26

## Summary

Cleared the unambiguously-dead items from the vestige sweep's §1. Of the five
candidates, verification before deletion caught **two false positives**, so only
two were actually removed.

## Removed

- **1.1 — stale brick build/lint configs.** Deleted
  `~/.pudl/schema/definitions/build.cue` and `lint.cue` — `brick.#Target` /
  `brick.#Interface` mu build/lint configs (not `#SystemModel` models). These were
  the only contents of the global `definitions/` dir, i.e. exactly what
  `pudl definition list` surfaced. **Outside git** (user's `~/.pudl`), so backed up
  to the session scratchpad before deletion.
- **1.2 — the dead `Vault` field.** Removed `SystemModel.Vault`
  (`internal/systemmodel/systemmodel.go`) and the `vault?:` field +
  comment (`internal/systemmodel/schema.cue`). The vault subsystem was removed in
  `bfaaf03`; the field was never read. Secrets are handled via `sealed_inputs` /
  `sealed_input_modes` on the populate arm, not `vault`.

## Rejected (false positives — kept)

- **1.4 — `cmd/migrate.go`.** NOT an empty stub. `cmd/identity_migrate.go:49`
  attaches `identityMigrateCmd` to `migrateCmd` — `pudl migrate identity` is the
  live command and `migrate.go` is its parent. Deleting it would break the command.
- **1.5 — `datalog.Compile` / `CompileWithOptions`.** `CompileWithOptions` is
  heavily used (`recursive.go:97,133`, `sql_eval.go:33`). Only the thin bare
  `Compile` wrapper (`compile.go:25`) is uncalled — a reasonable public convenience
  entry point, not worth removing.

## Deferred

- **1.3 — `entry_type='artifact'`/`'import'` taxonomy.** Entangled with Cluster A's
  standalone drift checker (`catalog_artifacts.go` is read by
  `internal/drift/checker.go`); to be removed with the Cluster A decision. See
  `docs/vestige-sweep.md` §5.2.

## Verification

- `CGO_ENABLED=0 go build ./...` — clean.
- `CGO_ENABLED=0 go test ./internal/systemmodel/... ./cmd/... ./...` — all pass
  (embedded `schema.cue` still compiles after the `vault?:` removal).
