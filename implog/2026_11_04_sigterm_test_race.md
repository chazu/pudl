# Deterministic SIGTERM cancellation test (pudl-o2u)

**Date:** 2026-11-04

## Root cause

`internal/proc` `TestCommandCancelSendsSIGTERM` cancelled the context 200 ms
after starting the fake tool, whether or not the tool's shell had installed its
`trap ... TERM` yet. Under load (exec of a freshly written script plus shell
startup) the shell was often not that far, so SIGTERM's default action killed it
before the trap existed, and the `terminated` marker was never written. The
failing runs end at exactly 0.20 s, the cancel time, which matches. With 32
copies of the test binary run in parallel, 313 of 320 runs failed this way.

`proc.Command` itself was correct; only the test was racy.

## Work done

- The fake tool now prints `ready` on stdout right after installing its trap.
  The test starts the command with a stdout pipe, reads that line, and only
  then cancels and waits. There is no fixed sleep, and no timeout was raised.

## Verification

- Same 32-way parallel stress: 0 of 320 runs failed (313 of 320 failed before).
- `go test ./internal/proc -run TestCommandCancelSendsSIGTERM -count=50`: pass.
- `CGO_ENABLED=0 go test -count=1 ./...` three times: all pass.

## Public API

None (test-only change).
