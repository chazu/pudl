# Test infrastructure hygiene

**Date:** 2026-10-05

## Summary

Test helpers no longer ship in the release binary, duplicated and dead test
helpers are gone, tests cannot touch the developer's real home directory, and
timing-sensitive assertions no longer flake under `-race`.

## Changes

### Test helpers out of the release binary

`internal/database/testutil.go` was a non-`_test.go` file importing `testing`
and testify, so both were linked into every `pudl` build. It is now
`internal/database/testutil_test.go`. A separate `dbtest` package was not
possible: the in-package `database` tests use these helpers, and a package
that imports `database` cannot be imported by `database`'s own tests (import
cycle). The unused `GenerateCorruptedEntries` was removed.

`binary_deps_test.go` (module root) runs `go list -deps .` and fails if the
binary depends on `testing` or any testify package.

### Duplicated and dead helpers

- Deleted `test/testutil/{database_suite,database_assertions,database_generators,mock_services}.go`:
  a second copy of the database suite, generators and assertions that
  nothing imported. `database_suite.go` also installed a SIGINT handler in
  `init()` that called `os.Exit(1)` in any test binary importing the package.
- `test/testutil/assertions.go` keeps only the four helpers in use
  (`AssertFileExists`, `AssertFileContains`, `AssertDirectoryExists`,
  `AssertErrorContains`); unused fixture and temp-dir helpers were removed.
- `test/integration/infrastructure` is still used by the integration tests and
  stays, minus its process-global cleanup registry and SIGINT/`os.Exit`
  `init()` (the workspace already comes from `t.TempDir()`) and four unused
  validators and two unused suite methods.
- Deleted `test/integration/workflows/{import_workflow,performance}_test.go`:
  all six tests called `t.Skip` unconditionally on their first line.

About 2,600 lines removed.

### HOME isolation

New `internal/testenv.RunWithIsolatedHome(m *testing.M) int` points `HOME` at a
fresh temp directory for the whole test binary and removes it afterwards.
`TestMain` files using it were added to every package whose tests can reach
`internal/config`, `internal/workspace`, `internal/init`, `internal/mubridge`,
`internal/doctor` or `pkg/factstore` (determined from `go list -test -deps`):
`cmd`, `internal/config`, `internal/doctor`, `internal/mubridge`,
`internal/repo`, `internal/workspace`, `pkg/factstore`. Tests that already call
`t.Setenv("HOME", ...)` keep their own value.

Note: a user-level `~/.gitignore` rule (`/pkg/`) ignores new files under
`pkg/`; `pkg/factstore/main_test.go` had to be added with `git add -f`.

### Timing

- Removed wall-clock assertions (`catalog_test.go` 10s batch add;
  `query_test.go` 100ms/200ms/500ms/2s query limits). Durations are still
  logged; `BenchmarkAddEntry` / `BenchmarkQueryEntries` measure speed.
- `TestGetLatestObserve` passes explicit, distinct timestamps through
  `addTestObserve(..., at time.Time)` instead of sleeping 10ms.
- `TestRunReportsRoundTripAndLatest` no longer sleeps: `LatestRunReport` orders
  by `created_at DESC, run_id DESC`, so ties are already deterministic.

Left alone as intentional: `facts_tx_test.go` (150ms sleep widens a race
window it is testing), `cmd/run_workspace_cleanup_test.go` (subprocess that
waits to be signalled), and `test/integration/infrastructure_test.go` (sleeps
10ms to give a timer a known lower bound).

## Public API

- `internal/testenv.RunWithIsolatedHome(m *testing.M) int` — test-only helper.
