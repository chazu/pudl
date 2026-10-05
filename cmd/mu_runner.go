package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/chazu/pudl/internal/proc"
)

// muRunner is the subprocess seam between PUDL's run policy and mu's execution
// engine, for the phases outside the convergence loop.
//
// `acute.Executor` already abstracts the converge loop's observe/plan/apply, but
// its production implementation — and every other phase — reached
// `exec.Command("mu", ...)` directly, so the acceptance matrix in the
// architecture report ("observe-only differential run", "observe-only inventory
// run", "converge to clean") could not run end to end without a real mu, a
// cluster, and a network. That is the gap this closes: the subprocess boundary is
// part of the run's domain behaviour, so it is an injectable contract rather than
// a call.
//
// It is passed explicitly rather than kept in a package variable. A swappable
// global would be a shared mutable seam that parallel tests race on, and the
// cost of a parameter is one word at four call sites.
type muRunner interface {
	// Observe runs `mu observe --json <target>` against a config file and returns
	// stdout. Machine output is on stdout, diagnostics on stderr (invariant 7),
	// and stderr is folded into the error on failure.
	Observe(configPath, target string) ([]byte, error)

	// Build runs `mu build [flags...] <target>` against a config file and returns
	// stdout.
	Build(configPath, target string, flags ...string) ([]byte, error)
}

// execMu is the production runner: it invokes the real mu binary, preserving
// exactly the argument order and error wrapping the direct calls used.
//
// It is bound to the invocation's context, so an interrupted run stops mu
// (SIGTERM, then SIGKILL after proc.DefaultGrace) instead of waiting on it, and
// a call cut short that way returns an error wrapping context.Canceled. A
// non-zero timeout bounds each individual mu invocation (--mu-timeout).
type execMu struct {
	ctx     context.Context
	timeout time.Duration
}

// newExecMu returns the production runner bound to ctx.
func newExecMu(ctx context.Context, timeout time.Duration) execMu {
	if ctx == nil {
		ctx = context.Background()
	}
	return execMu{ctx: ctx, timeout: timeout}
}

func (m execMu) Observe(configPath, target string) ([]byte, error) {
	out, err := m.run([]string{"observe", "--config", configPath, "--json", target})
	if err != nil {
		return nil, fmt.Errorf("mu observe %s: %w", target, err)
	}
	return out, nil
}

func (m execMu) Build(configPath, target string, flags ...string) ([]byte, error) {
	args := append([]string{"build"}, flags...)
	args = append(args, "--config", configPath, target)
	return m.run(args)
}

// run executes mu, returning stdout and folding stderr into any error.
func (m execMu) run(args []string) ([]byte, error) {
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return proc.Output(ctx, m.timeout, "mu", args...)
}
