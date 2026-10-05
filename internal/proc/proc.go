// Package proc runs external tools (mu, cue) so that a cancelled PUDL
// invocation stops them politely and never waits on them forever.
//
// A bare exec.Command ignores Ctrl-C entirely: the child keeps running while
// PUDL blocks in Wait. exec.CommandContext alone kills with SIGKILL, which gives
// mu no chance to write the receipt of an apply it was in the middle of. The
// commands built here ask first (SIGTERM) and only force (SIGKILL) after a
// grace period.
package proc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// DefaultGrace is how long a tool gets to exit after SIGTERM before it is
// killed. Long enough for mu to finish writing a manifest, short enough that a
// second look at the terminal does not find PUDL still hanging.
const DefaultGrace = 10 * time.Second

// Command returns name bound to ctx: when ctx ends the process receives
// SIGTERM, and is killed if it is still running grace later.
func Command(ctx context.Context, grace time.Duration, name string, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, name, args...)
	command.Cancel = func() error {
		return command.Process.Signal(syscall.SIGTERM)
	}
	command.WaitDelay = grace
	return command
}

// Output runs name with a bounded lifetime and returns its stdout. A non-zero
// timeout limits this one invocation. On failure stderr is folded into the
// error, and when the run was stopped because ctx (or the timeout) ended, the
// error wraps that context error, so callers can tell a cancelled tool from a
// failing one with errors.Is.
func Output(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	runCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	command := Command(runCtx, DefaultGrace, name, args...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if err == nil {
		return stdout.Bytes(), nil
	}
	detail := strings.TrimSpace(stderr.String())
	if ctxErr := Stopped(ctx, runCtx); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) && ctx.Err() == nil {
			ctxErr = fmt.Errorf("timed out after %s: %w", timeout, ctxErr)
		}
		if detail != "" {
			return nil, fmt.Errorf("%w: %s", ctxErr, detail)
		}
		return nil, ctxErr
	}
	if detail != "" {
		return nil, fmt.Errorf("%w: %s", err, detail)
	}
	return nil, err
}

// Stopped reports why a run was stopped by its context: the caller's own
// cancellation takes precedence over a per-invocation timeout. nil means the
// tool failed on its own.
func Stopped(parent, run context.Context) error {
	if err := parent.Err(); err != nil {
		return err
	}
	return run.Err()
}

// Available reports whether name is on PATH.
func Available(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// Cancelled reports whether err is the result of a cancelled context (the
// operator interrupted the run), as opposed to a timeout or a tool failure.
func Cancelled(err error) bool {
	return errors.Is(err, context.Canceled)
}
