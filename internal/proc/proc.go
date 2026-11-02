// Package proc runs external tools (populate commands, cue) so that a
// cancelled PUDL invocation stops them politely and never waits on them
// forever.
//
// A bare exec.Command ignores Ctrl-C entirely: the child keeps running while
// PUDL blocks in Wait. exec.CommandContext alone kills with SIGKILL, which gives
// the tool no chance to clean up. The commands built here ask first (SIGTERM)
// and only force (SIGKILL) after a grace period.
package proc

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
	"time"
)

// DefaultGrace is how long a tool gets to exit after SIGTERM before it is
// killed. Long enough for a tool to finish writing its output, short enough that a
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

// Stopped reports why a run was stopped by its context: the caller's own
// cancellation takes precedence over a per-invocation timeout. nil means the
// tool failed on its own.
func Stopped(parent, run context.Context) error {
	if err := parent.Err(); err != nil {
		return err
	}
	return run.Err()
}

// Cancelled reports whether err is the result of a cancelled context (the
// operator interrupted the run), as opposed to a timeout or a tool failure.
func Cancelled(err error) bool {
	return errors.Is(err, context.Canceled)
}
