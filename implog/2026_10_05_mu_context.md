# Cancellation, mu timeouts and the mu doctor check

**Date:** 2026-10-05

## Problem

No `context.Context` reached the run path. mu ran under a bare `exec.Command`,
so Ctrl-C killed PUDL while mu kept going, and the run row was left unfinished.
Only the reconcile workspace had a signal handler (`removeOnSignal`); populate,
ewe and ad-hoc workspaces leaked until the 24h sweep. Nothing checked the mu
version before a run; a mismatch surfaced mid-run as a plan-format error.

## What changed

- **`internal/proc` (new).** `Command(ctx, grace, name, args...)` binds a
  subprocess to ctx: cancellation sends SIGTERM, then SIGKILL after `grace`
  (`DefaultGrace` = 10s, via `exec.Cmd.Cancel`/`WaitDelay`). `Output(ctx,
  timeout, name, args...)` returns stdout, folds stderr into errors, and wraps
  `context.Canceled` or `context.DeadlineExceeded` when the context stopped the
  tool (a timeout reads `timed out after <d>`). `Cancelled(err)` tells an
  operator cancellation from a timeout or failure. `Available(name)` replaces
  scattered `exec.LookPath` calls.
- **Entry point.** `main.go` uses `signal.NotifyContext(SIGINT, SIGTERM)` and
  `cmd.ExecuteContext(ctx)`; `cmd.Execute()` remains as a background-context
  wrapper.
- **Run path.** `executeRun(ctx, opts, deps)`, `executeRunSet(ctx, ...)`,
  `resumeOperation(ctx, ...)`, `resumeRunSet(ctx, ...)` and
  `executePreparedMutationPlan(ctx, ...)` take the context. The production mu
  runner is `newExecMu(ctx, timeout)`, built by `defaultRunDeps(ctx,
  muTimeout)`; the `muRunner` interface is unchanged, so scripted test runners
  need nothing. `loadMuPluginInfo(ctx, name)` is bounded at 2 minutes;
  `muPluginInfoFrom(ctx)` adapts it for model inspection.
- **Truthful conclusions.** `failureStatus(err)` maps a cancellation to
  `cancelled` and everything else to `failed`. It is used by `runConclusion`
  (run row), `applyRunError` (report) and set member results. The converge loop
  already marks an interrupted apply as needs-verification (the write-ahead
  attempt marker), so a run cut short mid-apply concludes `cancelled`,
  needs-verification, verdict `unknown`. A run interrupted before its phases
  start stops with `run interrupted before its phases began`.
- **Run sets.** `runSetExecutionContext.ctx` stops the observe loop before the
  next member; `executePreparedMutationPlan` records unstarted mutating members
  `cancelled`. `runSetStatus(ctx, report)` gives the set status `cancelled` when
  an interrupt is why it did not succeed.
- **Workspace registry** (`cmd/run_workspace_cleanup.go`). `workspaces.track(dir)`
  registers a temporary directory and returns an idempotent release. Reconcile,
  populate, ewe and ad-hoc mu-root workspaces all register. On the first
  interrupt they are released by the run's ordinary defers; on a second
  interrupt `exitOnSecondSignal` removes all registered directories and exits
  130. This replaces `removeOnSignal`. The 24h stale-workspace sweep is kept.
- **`--mu-timeout`** (persistent on `run`, so also `run set` and `run resume`):
  per-mu-invocation limit, default 0 (none). An expiry is recorded `failed`.
- **cue subprocesses.** `module tidy/info/add` use `requireCue()` (one
  availability check instead of three `LookPath`s) and `cueCommand(ctx, dir,
  args...)`.
- **`pudl doctor`** gains a `mu` check (`internal/doctor/mu_check.go`):
  `CheckMu(ctx)` / `CheckMuBinary(ctx, binary)` run `mu version`, parse
  `mu vX.Y.Z`, and compare with `MinMuVersion` = `v0.3.5` (CI's `MU_VERSION`).
  Missing, unreadable, unrecognized or older → `warning` (mu is optional for
  imports and replays).

## Public API

No `pkg/` changes. Internal: `proc.Command`, `proc.Output`, `proc.Stopped`,
`proc.Available`, `proc.Cancelled`, `proc.DefaultGrace`; `doctor.CheckMu`,
`doctor.CheckMuBinary`, `doctor.MinMuVersion`; `cmd.ExecuteContext`.

## Deliberately not done

- No resume of an interrupted run (D1); a cancelled run is re-run.
- A second interrupt exits without finishing the run row; the next run of that
  model reports it as unfinished, which is the existing crash semantics.
- Checks still run after a cancelled converge concludes (they are local Datalog
  queries, not mu calls).
- `internal/init` and `internal/validator` still run `cue mod tidy` with a bare
  `exec.Command`; they are outside `cmd/` and run during initialization.

## Tests

- `internal/proc`: stdout/stderr folding; cancellation delivers SIGTERM and
  returns `context.Canceled` within the grace period; a tool ignoring SIGTERM is
  killed after the grace period; a timeout is `DeadlineExceeded`, not a
  cancellation.
- `cmd/run_cancel_test.go`: a fake `mu` on PATH blocks in apply; cancelling the
  context terminates it with SIGTERM, the converge loop reports
  needs-verification, the run row records `cancelled` + needs-verification, and
  no `pudl_run_*` workspace remains. A `--mu-timeout` expiry is `failed`. An
  interrupted mutating set starts no member and reports `cancelled`.
- `cmd/run_workspace_cleanup_test.go`: registry track/release/removeAll; the
  watcher returns without an interrupt; a subprocess test sends two SIGTERMs and
  checks the first leaves unwinding to the run, the second removes the
  workspace and exits 130.
- `internal/doctor/mu_check_test.go`: fake mu reporting current, newer, older,
  pre-release and garbage versions; absent mu.
