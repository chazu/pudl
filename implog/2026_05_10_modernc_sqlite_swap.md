# modernc SQLite Swap

**Date:** 2026-05-10

## Summary

Replaced `mattn/go-sqlite3` (CGo) with `modernc.org/sqlite` (pure Go) as SQLite driver.

## Changes

- `internal/database/catalog.go`: Import swap, driver name `"sqlite3"` → `"sqlite"`, pragma DSN format updated to `_pragma=X(Y)` style, added `busy_timeout(5000)`
- `internal/doctor/checks.go`: Import swap, driver name update
- `go.mod`: Removed `mattn/go-sqlite3`, added `modernc.org/sqlite` v1.50.0 + transitive deps

## Key Details

- modernc registers as driver `"sqlite"` (not `"sqlite3"`)
- Pragma DSN format: `?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)` (not `?_journal_mode=WAL`)
- `busy_timeout(5000)` must be set explicitly — mattn had implicit busy handling that modernc does not
- Builds with `CGO_ENABLED=0` — no C compiler required

## Test Results

879 passed, 2 failed (pre-existing schema name test), 10 skipped. All concurrency tests pass.
